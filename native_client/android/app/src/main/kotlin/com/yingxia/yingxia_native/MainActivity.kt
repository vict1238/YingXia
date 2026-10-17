// SPDX-License-Identifier: GPL-3.0-or-later
package com.yingxia.yingxia_native

import android.os.Handler
import android.os.Looper
import android.os.Bundle
import android.os.Build
import android.util.Log
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger
import core.Core
import go.Seq
import org.json.JSONObject

// The gomobile core runs in this process. No localhost service from a computer
// and no WebView/Chrome is needed on the phone.
class MainActivity : FlutterActivity() {
    companion object {
        // Serialize hosts across Activity recreation, not only within one host.
        private val worker = Executors.newSingleThreadExecutor()
        private val hosts = AtomicInteger()
        private val nextHost = AtomicInteger()
    }

    private val hostId = nextHost.incrementAndGet()
    private val mainHandler = Handler(Looper.getMainLooper())
    @Volatile private var destroyed = false

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i("YingXiaCore", "host_created id=$hostId live=${hosts.incrementAndGet()}")
        super.onCreate(savedInstanceState)
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        Seq.setContext(applicationContext)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "yingxia/core")
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "start" -> worker.execute {
                        if (destroyed) return@execute
                        try {
                            val descriptor = JSONObject(Core.start(File(filesDir, "yingxia").absolutePath))
                            val emulator = Build.HARDWARE == "ranchu" || Build.HARDWARE == "goldfish" ||
                                (Build.PRODUCT.startsWith("sdk_gphone") && Build.FINGERPRINT.startsWith("google/sdk_gphone"))
                            descriptor.put("emulator", emulator)
                            Log.i("YingXiaCore", "core_start_completed host=$hostId emulator=$emulator")
                            mainHandler.post { if (!destroyed) result.success(descriptor.toString()) }
                        } catch (error: Throwable) {
                            mainHandler.post { if (!destroyed) result.error("startup", "映匣后台未能启动", null) }
                        }
                    }
                    "stop" -> worker.execute {
                        val count = hosts.get()
                        val stop = count == 0 || (!destroyed && count == 1)
                        Log.i("YingXiaCore", "core_stop host=$hostId execute=$stop live=$count")
                        if (stop) Core.stop()
                        mainHandler.post { if (!destroyed) result.success(null) }
                    }
                    else -> result.notImplemented()
                }
            }
    }

    override fun onDestroy() {
        destroyed = true
        Log.i("YingXiaCore", "host_destroyed id=$hostId live=${hosts.decrementAndGet()} finishing=$isFinishing")
        if (isFinishing) worker.execute {
            // A replaced host must not close the new host's connection.
            if (hosts.get() == 0) Core.stop()
        }
        super.onDestroy()
    }
}
