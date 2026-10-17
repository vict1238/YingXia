// SPDX-License-Identifier: GPL-3.0-or-later
package com.yingxia.yingxia_native

import android.app.Activity
import android.app.UiModeManager
import android.content.Intent
import android.content.res.Configuration
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
        private val documentWorker = Executors.newSingleThreadExecutor()
        private val hosts = AtomicInteger()
        private val nextHost = AtomicInteger()
        private const val diagnosticRequest = 2202
        private const val diagnosticMaxBytes = 2 * 1024 * 1024
    }

    private val hostId = nextHost.incrementAndGet()
    private val mainHandler = Handler(Looper.getMainLooper())
    @Volatile private var destroyed = false
    private var diagnosticResult: MethodChannel.Result? = null
    private var diagnosticBytes: ByteArray? = null

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
                            val television = getSystemService(UiModeManager::class.java)
                                ?.currentModeType == Configuration.UI_MODE_TYPE_TELEVISION
                            descriptor.put("android_tv", television)
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
                    "saveDiagnosticReport" -> {
                        if (diagnosticResult != null) {
                            result.error("export_busy", "诊断报告正在保存", null)
                        } else {
                            val json = (call.arguments as? Map<*, *>)?.get("json") as? String
                            try {
                                if (destroyed || json == null || json.length > diagnosticMaxBytes) {
                                    throw IllegalArgumentException()
                                }
                                JSONObject(json)
                                val bytes = json.toByteArray(Charsets.UTF_8)
                                if (bytes.size > diagnosticMaxBytes) throw IllegalArgumentException()
                                diagnosticResult = result
                                diagnosticBytes = bytes
                                @Suppress("DEPRECATION")
                                startActivityForResult(
                                    Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
                                        addCategory(Intent.CATEGORY_OPENABLE)
                                        type = "application/json"
                                        putExtra(Intent.EXTRA_TITLE, "yingxia_diagnostics.json")
                                    },
                                    diagnosticRequest,
                                )
                            } catch (error: Throwable) {
                                diagnosticResult = null
                                diagnosticBytes = null
                                result.error("export_unavailable", "无法打开诊断报告保存位置", null)
                            }
                        }
                    }
                    else -> result.notImplemented()
                }
            }
    }

    @Deprecated("Android activity-result callback retained for the FlutterActivity host")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != diagnosticRequest) return
        val result = diagnosticResult ?: return
        val bytes = diagnosticBytes
        val uri = data?.data
        if (resultCode != Activity.RESULT_OK || uri == null) {
            diagnosticResult = null
            diagnosticBytes = null
            result.success(false)
            return
        }
        if (uri.scheme != "content" || bytes == null) {
            diagnosticResult = null
            diagnosticBytes = null
            result.error("export_destination", "无法保存诊断报告", null)
            return
        }
        // The Storage Access Framework grants this single document URI. No
        // storage permission, caller-chosen path, or persistent URI grant exists.
        documentWorker.execute {
            try {
                contentResolver.openOutputStream(uri, "wt")?.use { stream ->
                    stream.write(bytes)
                    stream.flush()
                } ?: throw IllegalStateException()
                mainHandler.post {
                    if (!destroyed && diagnosticResult === result) {
                        diagnosticResult = null
                        diagnosticBytes = null
                        result.success(true)
                    }
                }
            } catch (error: Throwable) {
                mainHandler.post {
                    if (!destroyed && diagnosticResult === result) {
                        diagnosticResult = null
                        diagnosticBytes = null
                        result.error("export_write", "无法保存诊断报告，请重新选择位置", null)
                    }
                }
            }
        }
    }

    override fun onDestroy() {
        destroyed = true
        diagnosticResult?.success(false)
        diagnosticResult = null
        diagnosticBytes = null
        Log.i("YingXiaCore", "host_destroyed id=$hostId live=${hosts.decrementAndGet()} finishing=$isFinishing")
        if (isFinishing) worker.execute {
            // A replaced host must not close the new host's connection.
            if (hosts.get() == 0) Core.stop()
        }
        super.onDestroy()
    }
}
