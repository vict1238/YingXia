// This file is a part of media_kit
// (https://github.com/media-kit/media-kit).
//
// Copyright © 2021 & onwards, Hitesh Kumar Saini <saini123hitesh@gmail.com>.
// All rights reserved.
// Use of this source code is governed by MIT license that can be found in the
// LICENSE file.

#include "video_output_manager.h"

VideoOutputManager::VideoOutputManager(
    flutter::PluginRegistrarWindows* registrar)
    : registrar_(registrar) {}

void VideoOutputManager::Create(
    int64_t handle,
    VideoOutputConfiguration configuration,
    std::function<void(int64_t, int64_t, int64_t)> texture_update_callback) {
  std::shared_ptr<std::atomic<VideoRendererBackend>> renderer_state;
  {
    std::lock_guard<std::mutex> lock(renderer_states_mutex_);
    auto entry = renderer_states_.try_emplace(
        handle, std::make_shared<std::atomic<VideoRendererBackend>>(
                    VideoRendererBackend::kPending)).first;
    renderer_state = entry->second;
  }
  control_pool_->Post([=]() {
    std::lock_guard<std::mutex> lock(mutex_);
    if (video_outputs_.find(handle) == video_outputs_.end()) {
      auto instance = std::make_unique<VideoOutput>(
          handle, configuration, registrar_, thread_pool_.get(), renderer_state);
      instance->SetTextureUpdateCallback(texture_update_callback);
      video_outputs_.insert(std::make_pair(handle, std::move(instance)));
    }
  });
}

std::string VideoOutputManager::GetRendererBackend(int64_t handle) {
  std::unique_lock<std::mutex> lock(renderer_states_mutex_, std::try_to_lock);
  if (!lock.owns_lock()) {
    return "pending";
  }
  auto entry = renderer_states_.find(handle);
  if (entry == renderer_states_.end()) {
    return "pending";
  }
  switch (entry->second->load()) {
    case VideoRendererBackend::kHardware:
      return "hardware";
    case VideoRendererBackend::kSoftware:
      return "software";
    default:
      return "pending";
  }
}

void VideoOutputManager::SetSize(int64_t handle,
                                 std::optional<int64_t> width,
                                 std::optional<int64_t> height) {
  control_pool_->Post([=]() {
    std::lock_guard<std::mutex> lock(mutex_);
    if (video_outputs_.find(handle) != video_outputs_.end()) {
      video_outputs_[handle]->SetSize(width, height);
    }
  });
}

void VideoOutputManager::Dispose(int64_t handle, std::function<void()> completion) {
  control_pool_->Post([=]() {
    {
      std::lock_guard<std::mutex> lock(mutex_);
      if (video_outputs_.find(handle) != video_outputs_.end()) {
        // VideoOutput's destructor waits for texture unregister and renderer
        // cleanup, including mpv_render_context_free with its GL context.
        video_outputs_.erase(handle);
      }
    }
    // Release the manager lock before posting the MethodChannel reply. Dart
    // may destroy the mpv core only after this completion has been delivered.
    completion();
  });
}

VideoOutputManager::~VideoOutputManager() {
  // Retire outputs after every previously accepted control operation, then
  // join this queue before destroying the manager or render worker. No old
  // request can outlive its Dispose and affect a subsequent reused handle.
  control_pool_->Post([this]() {
    std::lock_guard<std::mutex> lock(mutex_);
    video_outputs_.clear();
  }).wait();
  control_pool_.reset();
}
