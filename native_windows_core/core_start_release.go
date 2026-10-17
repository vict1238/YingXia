// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !native_fixture

package core

import "yingxia.local/nativecore/internal/engine"

func startRuntime(dir string) (*engine.Runtime, error) { return engine.NewRuntime(dir) }
