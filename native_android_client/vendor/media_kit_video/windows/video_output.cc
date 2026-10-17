// This file is a part of media_kit
// (https://github.com/media-kit/media-kit).
//
// Copyright © 2021 & onwards, Hitesh Kumar Saini <saini123hitesh@gmail.com>.
// All rights reserved.
// Use of this source code is governed by MIT license that can be found in the
// LICENSE file.

#include "video_output.h"

#include <algorithm>

// Limit the frame size to 1080p in software rendering.
// This is for performance reasons & to avoid allocating too much memory.
#define SW_RENDERING_MAX_WIDTH 1920
#define SW_RENDERING_MAX_HEIGHT 1080
#define SW_RENDERING_PIXEL_BUFFER_SIZE \
  (SW_RENDERING_MAX_WIDTH) * (SW_RENDERING_MAX_HEIGHT) * (4)

VideoOutput::VideoOutput(int64_t handle,
                         VideoOutputConfiguration configuration,
                         flutter::PluginRegistrarWindows* registrar,
                         ThreadPool* thread_pool_ref,
                         std::shared_ptr<std::atomic<VideoRendererBackend>> renderer_state)
    : handle_(reinterpret_cast<mpv_handle*>(handle)),
      width_(configuration.width),
      height_(configuration.height),
      configuration_(configuration),
      registrar_(registrar),
      thread_pool_ref_(thread_pool_ref),
      renderer_state_(std::move(renderer_state)) {
  // Normal core APIs must not run on the render worker with advanced control.
  // The constructor itself is on the manager's independent creation thread.
  // No render context for this core exists yet.
  mpv_set_option_string(handle_, "video-sync", "audio");
  mpv_set_option_string(handle_, "video-timing-offset", "0");
  // The constructor must be invoked through the thread pool, because
  // |ANGLESurfaceManager| & libmpv render context creation can conflict with
  // the existing |Render| or |Resize| calls from another |VideoOutput|
  // instances (which will result in access violation).
  auto future = thread_pool_ref_->Post([&]() {
    int advanced_control = 1;
    // First try to initialize video playback with hardware acceleration &
    // |ANGLESurfaceManager|, use S/W API as fallback.
    auto is_hardware_acceleration_enabled = false;
    // Attempt to use H/W rendering.
    if (configuration.enable_hardware_acceleration) {
      try {
        // OpenGL context needs to be set before |mpv_render_context_create|.
        surface_manager_ = std::make_unique<ANGLESurfaceManager>(
            static_cast<int32_t>(width_.value_or(1)),
            static_cast<int32_t>(height_.value_or(1)));
        surface_manager_->MakeCurrent(true);
        mpv_opengl_init_params gl_init_params{
            [](auto, auto name) {
              return reinterpret_cast<void*>(eglGetProcAddress(name));
            },
            nullptr,
        };
        mpv_render_param params[] = {
            {MPV_RENDER_PARAM_API_TYPE, MPV_RENDER_API_TYPE_OPENGL},
            {MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &gl_init_params},
            {MPV_RENDER_PARAM_ADVANCED_CONTROL, &advanced_control},
            {MPV_RENDER_PARAM_INVALID, nullptr},
        };
        // Create render context.
        if (mpv_render_context_create(&render_context_, handle_, params) == 0) {
          mpv_render_context_set_update_callback(
              render_context_,
              [](void* context) {
                // Queue only. A control request can arrive without a frame;
                // ProcessUpdates services it on the independent render worker.
                auto that = reinterpret_cast<VideoOutput*>(context);
                that->NotifyRender();
              },
              reinterpret_cast<void*>(this));
          // Install the update callback immediately after context creation.
          // Publish a Flutter GPU texture only after the renderer exists.
          Resize(width_.value_or(1), height_.value_or(1));
          // Set flag to true, indicating that H/W rendering is supported.
          is_hardware_acceleration_enabled = true;
          renderer_state_->store(VideoRendererBackend::kHardware);
          std::cout << "media_kit: VideoOutput: Using H/W rendering."
                    << std::endl;
          std::cout << "yingxia: VideoOutput: Advanced render control enabled."
                    << std::endl;
        }
      } catch (...) {
        // Likely received an |std::runtime_error| from |ANGLESurfaceManager|,
        // which indicates that H/W rendering is not supported.
        if (render_context_) {
          mpv_render_context_set_update_callback(render_context_, nullptr,
                                                 nullptr);
          if (surface_manager_) {
            surface_manager_->MakeCurrent(true);
          }
          mpv_render_context_free(render_context_);
          render_context_ = nullptr;
        }
      }
    }
    if (!is_hardware_acceleration_enabled) {
      // A failed hardware setup must not leave the software path using an
      // unrelated GL surface/context.
      surface_manager_.reset(nullptr);
      std::cout << "media_kit: VideoOutput: Using S/W rendering." << std::endl;
      // Allocate a "large enough" buffer ahead of time.
      pixel_buffer_ =
          std::make_unique<uint8_t[]>(SW_RENDERING_PIXEL_BUFFER_SIZE);
      Resize(width_.value_or(1), height_.value_or(1));
      mpv_render_param params[] = {
          {MPV_RENDER_PARAM_API_TYPE, MPV_RENDER_API_TYPE_SW},
          {MPV_RENDER_PARAM_ADVANCED_CONTROL, &advanced_control},
          {MPV_RENDER_PARAM_INVALID, nullptr},
      };
      if (mpv_render_context_create(&render_context_, handle_, params) == 0) {
        mpv_render_context_set_update_callback(
            render_context_,
            [](void* context) {
              // Notify Flutter that a new frame is available. The actual
              // rendering will take place in the |Render| method, which will be
              // called by Flutter on the render thread.
              auto that = reinterpret_cast<VideoOutput*>(context);
              that->NotifyRender();
            },
            reinterpret_cast<void*>(this));
      }
      renderer_state_->store(VideoRendererBackend::kSoftware);
    }
  });
  future.wait();
}

VideoOutput::~VideoOutput() {
  destroyed_ = true;
  renderer_state_->store(VideoRendererBackend::kPending);
  {
    // Old texture unregister callbacks take this independently-lived guard
    // before testing alive_. Closing it waits for callbacks using this object.
    std::lock_guard<std::mutex> lock(*callback_mutex_);
    alive_->store(false);
  }
  // This function explicitly accesses no GL state and may run on the manager
  // thread. Taking mpv's callback lock also waits for an in-flight callback,
  // so no new task referencing this object can be posted after the barrier.
  if (render_context_) {
    mpv_render_context_set_update_callback(render_context_, nullptr, nullptr);
  }
  // A resize already in progress can still publish its final registered ID.
  // Drain it before selecting the one final texture to unregister; otherwise
  // teardown could unregister an old ID and leave the new registration live.
  thread_pool_ref_->Post([]() {}).wait();
  auto promise = std::promise<void>();
  auto cleanup = [&]() {
    // All prior update/resize tasks have finished or observed destroyed_.
    // Freeing the render context may process pending decoder/control dispatch
    // itself. Keep its original GL context current until mpv has released it.
    if (surface_manager_) {
      surface_manager_->MakeCurrent(true);
    }
    if (render_context_) {
      mpv_render_context_free(render_context_);
      render_context_ = nullptr;
    }
    if (surface_manager_) {
      surface_manager_->MakeCurrent(false);
    }
    {
      std::lock_guard<std::mutex> lock(textures_mutex_);
      texture_variants_.clear();
      textures_.clear();
      pixel_buffer_textures_.clear();
      surface_manager_.reset(nullptr);
    }
    promise.set_value();
  };
  if (texture_id_) {
    registrar_->texture_registrar()->UnregisterTexture(
        texture_id_, [&, texture_id = texture_id_, cleanup]() {
          // Add one more task into the thread pool queue & exit the destructor
          // only when it gets executed. This will ensure that all the tasks
          // posted to the thread pool i.e. render or resize before this are
          // executed (and won't reference the dead object anymore), most
          // notably |CheckAndResize| & |Render|.
          thread_pool_ref_->Post([&, id = texture_id, cleanup]() {
            std::cout << "media_kit: VideoOutput: Free Texture: " << id
                      << std::endl;
            std::cout << "VideoOutput::~VideoOutput: "
                      << reinterpret_cast<int64_t>(handle_) << std::endl;
            cleanup();
          });
        });
  } else {
    thread_pool_ref_->Post(cleanup);
  }

  promise.get_future().wait();
  texture_id_ = 0;

}

void VideoOutput::NotifyRender() {
  if (destroyed_) {
    return;
  }
  // Every callback queues an update, including callbacks raised while another
  // update is running. No callback performs an mpv/render call inline.
  thread_pool_ref_->Post([this]() { ProcessUpdates(); });
}

void VideoOutput::ProcessUpdates(bool force_render) {
  if (destroyed_ || !render_context_) {
    return;
  }
  if (surface_manager_) {
    surface_manager_->MakeCurrent(true);
  }
  auto flags = mpv_render_context_update(render_context_);
  if (surface_manager_) {
    surface_manager_->MakeCurrent(false);
  }
  if ((flags & MPV_RENDER_UPDATE_FRAME) || force_render) {
    CheckAndResize();
    Render();
  }
}

void VideoOutput::Render() {
  if (!destroyed_ && render_context_ && texture_id_) {
    // H/W
    if (surface_manager_ != nullptr) {
      surface_manager_->Draw([&]() {
        mpv_opengl_fbo fbo{
            0,
            surface_manager_->width(),
            surface_manager_->height(),
            0,
        };
        mpv_render_param params[]{
            {MPV_RENDER_PARAM_OPENGL_FBO, &fbo},
            {MPV_RENDER_PARAM_INVALID, nullptr},
        };
        mpv_render_context_render(render_context_, params);
      });
    }
    // S/W
    if (pixel_buffer_ != nullptr) {
      int32_t size[]{
          static_cast<int32_t>(pixel_buffer_textures_.at(texture_id_)->width),
          static_cast<int32_t>(pixel_buffer_textures_.at(texture_id_)->height),
      };
      size_t pitch = 4 * static_cast<size_t>(size[0]);
      mpv_render_param params[]{
          {MPV_RENDER_PARAM_SW_SIZE, size},
          {MPV_RENDER_PARAM_SW_FORMAT, "rgb0"},
          {MPV_RENDER_PARAM_SW_STRIDE, &pitch},
          {MPV_RENDER_PARAM_SW_POINTER, pixel_buffer_.get()},
          {MPV_RENDER_PARAM_INVALID, nullptr},
      };
      mpv_render_context_render(render_context_, params);
    }
    try {
      // Notify Flutter that a new frame is available.
      registrar_->texture_registrar()->MarkTextureFrameAvailable(texture_id_);
    } catch (...) {
      // Prevent any redundant exceptions if the texture is unregistered etc.
    }
  }
}

void VideoOutput::SetTextureUpdateCallback(
    std::function<void(int64_t, int64_t, int64_t)> callback) {
  texture_update_callback_ = callback;
  texture_update_callback_(texture_id_, GetVideoWidth(), GetVideoHeight());
}

void VideoOutput::SetSize(std::optional<int64_t> width,
                          std::optional<int64_t> height) {
  thread_pool_ref_->Post([&, width, height]() {
    if (destroyed_) {
      return;
    }
    if (width.has_value()) {
      // H/W
      if (surface_manager_ != nullptr) {
        width_ = width.value();
      }
      // S/W
      if (pixel_buffer_ != nullptr) {
        // Limit width if software rendering is being used.
        width_ = std::clamp(width.value(), static_cast<int64_t>(0),
                            static_cast<int64_t>(SW_RENDERING_MAX_WIDTH));
      }
    } else {
      width_ = std::nullopt;
    }
    if (height.has_value()) {
      // H/W
      if (surface_manager_ != nullptr) {
        height_ = height.value();
      }
      // S/W
      if (pixel_buffer_ != nullptr) {
        // Limit width if software rendering is being used.
        height_ = std::clamp(height.value(), static_cast<int64_t>(0),
                             static_cast<int64_t>(SW_RENDERING_MAX_HEIGHT));
      }
    } else {
      height_ = std::nullopt;
    }
    // Redraw a paused current frame into the new target, and service pending
    // control dispatch before doing so. Resizing does not call core APIs.
    ProcessUpdates(true);
  });
}

void VideoOutput::CheckAndResize() {
  // Check if a new texture with different dimensions is needed.
  auto required_width = GetVideoWidth(), required_height = GetVideoHeight();
  if (required_width < 1 || required_height < 1) {
    // Invalid.
    return;
  }
  int64_t current_width = -1, current_height = -1;
  if (surface_manager_ != nullptr) {
    current_width = surface_manager_->width();
    current_height = surface_manager_->height();
  }
  if (pixel_buffer_ != nullptr) {
    current_width = pixel_buffer_textures_.at(texture_id_)->width;
    current_height = pixel_buffer_textures_.at(texture_id_)->height;
  }
  // Currently rendered video output dimensions.
  // Either H/W or S/W rendered.
  assert(current_width > 0);
  assert(current_height > 0);
  if (required_width == current_width && required_height == current_height) {
    // No creation of new texture required.
    return;
  }
  Resize(required_width, required_height);
}

void VideoOutput::Resize(int64_t required_width, int64_t required_height) {
  if (destroyed_) {
    return;
  }
  std::cout << required_width << " " << required_height << std::endl;
  // Retire the current ID under the same mutex used by texture callbacks.
  // Registration APIs stay outside this lock because Flutter can invoke a
  // callback synchronously. Preserve the old exported resource until its
  // unregister completion, including readers that already received a HANDLE.
  int64_t retired_id = 0;
  Microsoft::WRL::ComPtr<ID3D11Texture2D> retired_texture;
  {
    std::lock_guard<std::mutex> lock(textures_mutex_);
    retired_id = texture_id_;
    texture_id_ = 0;
    if (retired_id && surface_manager_) {
      retired_texture = surface_manager_->RetainSharedTexture();
    }
  }
  if (retired_id) {
    registrar_->texture_registrar()->UnregisterTexture(
        retired_id, [&, id = retired_id, alive = alive_,
                     guard = callback_mutex_, lease = retired_texture]() {
          (void)lease;  // Holds the exported D3D resource until completion.
          std::lock_guard<std::mutex> callback_lock(*guard);
          if (!alive->load()) {
            return;
          }
          if (id) {
            std::cout << "media_kit: VideoOutput: Free Texture: " << id
                      << std::endl;
            std::lock_guard<std::mutex> lock(textures_mutex_);
            if (destroyed_) {
              return;
            }
            if (texture_variants_.find(id) != texture_variants_.end()) {
              texture_variants_.erase(id);
            }
            // H/W
            if (textures_.find(id) != textures_.end()) {
              textures_.erase(id);
            }
            // S/W
            if (pixel_buffer_textures_.find(id) !=
                pixel_buffer_textures_.end()) {
              pixel_buffer_textures_.erase(id);
            }
          }
        });
  }
  // H/W
  if (surface_manager_ != nullptr) {
    // Destroy internal ID3D11Texture2D & EGLSurface & create new with updated
    // dimensions while preserving previous EGLDisplay & EGLContext.
    surface_manager_->SetSize(static_cast<int32_t>(required_width),
                              static_cast<int32_t>(required_height));
    auto texture = std::make_unique<FlutterDesktopGpuSurfaceDescriptor>();
    texture->struct_size = sizeof(FlutterDesktopGpuSurfaceDescriptor);
    texture->handle = surface_manager_->handle();
    texture->width = texture->visible_width = surface_manager_->width();
    texture->height = texture->visible_height = surface_manager_->height();
    texture->release_context = nullptr;
    texture->release_callback = [](void*) {};
    texture->format = kFlutterDesktopPixelFormatBGRA8888;
    auto registration_id = std::make_shared<int64_t>(0);
    auto texture_variant =
        std::make_unique<flutter::TextureVariant>(flutter::GpuSurfaceTexture(
            kFlutterDesktopGpuSurfaceTypeDxgiSharedHandle,
            [this, registration_id, alive = alive_, guard = callback_mutex_](auto, auto) {
              std::lock_guard<std::mutex> callback_lock(*guard);
              if (!alive->load()) {
                return (FlutterDesktopGpuSurfaceDescriptor*)nullptr;
              }
              std::lock_guard<std::mutex> lock(textures_mutex_);
              auto entry = textures_.find(*registration_id);
              if (*registration_id == 0 || *registration_id != texture_id_ ||
                  entry == textures_.end() || !surface_manager_) {
                return (FlutterDesktopGpuSurfaceDescriptor*)nullptr;
              }
              surface_manager_->Read();
              return entry->second.get();
            }));
    // Register outside the callback mutex. Before publication callbacks see
    // registration_id==0 and return null rather than indexing an absent map.
    auto new_id =
        registrar_->texture_registrar()->RegisterTexture(texture_variant.get());
    std::cout << "media_kit: VideoOutput: Create Texture: " << new_id
              << std::endl;
    {
      std::lock_guard<std::mutex> lock(textures_mutex_);
      textures_.emplace(std::make_pair(new_id, std::move(texture)));
      texture_variants_.emplace(
          std::make_pair(new_id, std::move(texture_variant)));
      *registration_id = new_id;
      texture_id_ = new_id;
    }
    // Notify public texture update callback.
    texture_update_callback_(new_id, required_width, required_height);
  }
  // S/W
  if (pixel_buffer_ != nullptr) {
    auto pixel_buffer_texture = std::make_unique<FlutterDesktopPixelBuffer>();
    pixel_buffer_texture->buffer = pixel_buffer_.get();
    pixel_buffer_texture->width = required_width;
    pixel_buffer_texture->height = required_height;
    pixel_buffer_texture->release_context = nullptr;
    pixel_buffer_texture->release_callback = [](void*) {};
    auto registration_id = std::make_shared<int64_t>(0);
    auto texture_variant = std::make_unique<flutter::TextureVariant>(
        flutter::PixelBufferTexture(
            [this, registration_id, alive = alive_, guard = callback_mutex_](auto, auto) {
          std::lock_guard<std::mutex> callback_lock(*guard);
          if (!alive->load()) {
            return (FlutterDesktopPixelBuffer*)nullptr;
          }
          std::lock_guard<std::mutex> lock(textures_mutex_);
          auto entry = pixel_buffer_textures_.find(*registration_id);
          if (*registration_id == 0 || *registration_id != texture_id_ ||
              entry == pixel_buffer_textures_.end()) {
            return (FlutterDesktopPixelBuffer*)nullptr;
          }
          return entry->second.get();
        }));
    // Register new texture.
    auto new_id =
        registrar_->texture_registrar()->RegisterTexture(texture_variant.get());
    std::cout << "media_kit: VideoOutput: Create Texture: " << new_id
              << std::endl;
    {
      std::lock_guard<std::mutex> lock(textures_mutex_);
      pixel_buffer_textures_.emplace(
          std::make_pair(new_id, std::move(pixel_buffer_texture)));
      texture_variants_.emplace(
          std::make_pair(new_id, std::move(texture_variant)));
      *registration_id = new_id;
      texture_id_ = new_id;
    }
    // Notify public texture update callback.
    texture_update_callback_(new_id, required_width, required_height);
  }
}

int64_t VideoOutput::GetVideoWidth() {
  // Dart's metadata listener supplies rotation-correct source dimensions or
  // an explicit FSR target. Never synchronously query the mpv core here: a
  // vo-passes request could be waiting for this worker's update dispatch.
  return width_.value_or(1);
}

int64_t VideoOutput::GetVideoHeight() {
  return height_.value_or(1);
}
