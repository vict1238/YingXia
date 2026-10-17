// SPDX-License-Identifier: GPL-3.0-or-later
// Isolated scheduling checks using the actual vendored ThreadPool used by
// VideoOutputManager. No Flutter window, media, GPU or user data is involved.
#include "../vendor/media_kit_video/windows/thread_pool.h"

#include <atomic>
#include <chrono>
#include <iostream>
#include <stdexcept>
#include <string>
#include <vector>

void require(bool condition, const char* message) {
  if (!condition) throw std::runtime_error(message);
}

int main() {
  ThreadPool render(1);
  ThreadPool control(1);
  std::thread::id render_id, control_id;
  render.Post([&]() { render_id = std::this_thread::get_id(); }).wait();
  control.Post([&]() { control_id = std::this_thread::get_id(); }).wait();
  require(render_id != control_id, "control and render worker must differ");

  // Simulate renderer initialization occupying the control worker. Size
  // requests must still be accepted before that worker becomes available.
  std::promise<void> creation_started, release_creation;
  auto release = release_creation.get_future().share();
  std::atomic<int> width{0};
  std::vector<int> applied;
  control.Post([&]() {
    creation_started.set_value();
    release.wait();
    render.Post([&]() { width = 1280; }).wait();
  });
  creation_started.get_future().wait();
  control.Post([&]() {
    render.Post([&]() {
      std::this_thread::sleep_for(std::chrono::milliseconds(35));
      width = 1920;
      applied.push_back(1920);
    });
  });
  control.Post([&]() {
    render.Post([&]() { width = 1280; applied.push_back(1280); });
  });
  // This line being reached proves posting did not wait for initialization.
  release_creation.set_value();
  control.Post([]() {}).wait();
  render.Post([]() {}).wait();
  require(applied == std::vector<int>({1920,1280}), "older Enhanced target overtook later Off target");
  require(width == 1280, "Off must retain the final source-size target");

  // Disposal must drain previous rendering before the same handle is reused.
  // Old generation work cannot apply after the replacement creation.
  std::atomic<int> generation{1};
  std::vector<std::pair<int,int>> generations;
  control.Post([&]() {
    render.Post([&]() {
      std::this_thread::sleep_for(std::chrono::milliseconds(20));
      require(generation == 1, "old size crossed output lifetime boundary");
      generations.emplace_back(1,1920);
    });
  });
  control.Post([&]() {
    render.Post([&]() { generation = 0; }).wait();
  });
  control.Post([&]() {
    render.Post([&]() {
      require(generation == 0, "new creation ran before disposal completed");
      generation = 2;
    }).wait();
  });
  control.Post([&]() {
    render.Post([&]() {
      require(generation == 2, "replacement target applied to old output");
      generations.emplace_back(2,1280);
    });
  });
  control.Post([]() {}).wait();
  render.Post([]() {}).wait();
  require(generations == std::vector<std::pair<int,int>>({{1,1920},{2,1280}}), "handle reuse lost ordered lifecycle");

  // Rapid requested dimensions exercise FIFO acceptance under queue load.
  applied.clear();
  for (int i=0;i<1000;i++) {
    control.Post([&,i]() { render.Post([&,i]() { applied.push_back(i); }); });
  }
  control.Post([]() {}).wait();
  render.Post([]() {}).wait();
  require(applied.size() == 1000, "queued sizes lost");
  for (int i=0;i<1000;i++) require(applied[i] == i, "queued sizes reordered");
  std::cout << "{\"passed\":true,\"checks\":[\"nonblocking receipt during initialization\",\"delayed Enhanced then Off order\",\"Dispose and reused-handle lifetime order\",\"1000 FIFO control/render requests\"],\"scope\":\"actual vendored ThreadPool scheduling; no GUI/GPU playback\"}" << std::endl;
}
