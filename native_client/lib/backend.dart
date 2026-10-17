// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/services.dart';

/// Private transport shared by the Windows child process and Android JNI core.
/// Construction has no side effects, so tests can replace this connection.
class BackendConnection {
  static const _channel = MethodChannel('yingxia/core');
  HttpClient? _client;
  Process? _process;
  StreamSubscription<String>? _stdout;
  StreamSubscription<List<int>>? _stderr;
  Future<void>? _starting;
  String _origin = '';
  String _cookie = '';
  String _csrf = '';
  bool _disposed = false;
  bool _isAndroidEmulator = false;

  String get origin => _origin;
  bool get isAndroidEmulator => _isAndroidEmulator;
  Map<String, String> get mediaHeaders => {
    'Cookie': _cookie,
    'Origin': _origin,
  };

  Future<void> start() {
    if (_disposed) return Future.error(StateError('连接已关闭'));
    if (_origin.isNotEmpty) return Future.value();
    return _starting ??= _start().catchError((
      Object error,
      StackTrace stack,
    ) async {
      _origin = '';
      _cookie = '';
      _csrf = '';
      _isAndroidEmulator = false;
      _client?.close(force: true);
      _client = null;
      _process?.kill();
      try {
        await _process?.stdin.close();
      } catch (_) {}
      await _stdout?.cancel();
      await _stderr?.cancel();
      _process = null;
      _stdout = null;
      _stderr = null;
      if (Platform.isAndroid) {
        try {
          await _channel.invokeMethod<void>('stop');
        } catch (_) {}
      }
      _starting = null;
      Error.throwWithStackTrace(error, stack);
    });
  }

  Future<void> _start() async {
    if (_disposed) throw StateError('连接已关闭');
    String? descriptor;
    if (Platform.isAndroid) {
      descriptor = await _channel.invokeMethod<String>('start');
    } else if (Platform.isWindows) {
      final runnerDir = File(Platform.resolvedExecutable).parent.path;
      final separator = Platform.pathSeparator;
      final override = Platform.environment['YINGXIA_NATIVE_BACKEND'];
      final candidates = [
        if (override != null && override.isNotEmpty) override,
        '$runnerDir${separator}yingxia_backend.exe',
        '$runnerDir${separator}backend${separator}yingxia_backend.exe',
        '${Directory.current.path}$separator..${separator}native_core${separator}build${separator}yingxia_backend.exe',
      ];
      String? executable;
      for (final candidate in candidates) {
        if (await File(candidate).exists()) {
          executable = File(candidate).absolute.path;
          break;
        }
      }
      if (executable == null) throw Exception('缺少映匣后台程序，请重新解压完整安装包');
      final dataDir =
          Platform.environment['YINGXIA_DATA_DIR'] ??
          '${Platform.environment['LOCALAPPDATA'] ?? Directory.current.path}${separator}YingXia${separator}standalone';
      _process = await Process.start(executable, ['--data-dir', dataDir]);
      final firstLine = Completer<String>();
      _stdout = _process!.stdout
          .transform(utf8.decoder)
          .transform(const LineSplitter())
          .listen(
            (line) {
              if (!firstLine.isCompleted) firstLine.complete(line);
            },
            onError: (Object error) {
              if (!firstLine.isCompleted) firstLine.completeError(error);
            },
            onDone: () {
              if (!firstLine.isCompleted) {
                firstLine.completeError(Exception('映匣后台未能启动'));
              }
            },
          );
      _stderr = _process!.stderr.listen((_) {});
      descriptor = await firstLine.future.timeout(const Duration(seconds: 20));
    } else {
      throw UnsupportedError('当前版本支持 Windows 和 Android');
    }
    if (descriptor == null || descriptor.isEmpty) throw Exception('映匣后台未能启动');
    final bootstrap = jsonDecode(descriptor) as Map<String, dynamic>;
    if (bootstrap['error'] != null) throw Exception(bootstrap['error']);
    final uri = Uri.tryParse(bootstrap['origin'] as String? ?? '');
    final cookie = bootstrap['cookie'] as String? ?? '';
    final csrf = bootstrap['csrf'] as String? ?? '';
    if (uri == null ||
        uri.scheme != 'http' ||
        uri.host != '127.0.0.1' ||
        uri.port <= 0 ||
        uri.path.isNotEmpty ||
        uri.hasQuery ||
        uri.hasFragment ||
        uri.userInfo.isNotEmpty ||
        !RegExp(r'^yx_session_\d+=[a-f0-9]{64}$').hasMatch(cookie) ||
        !RegExp(r'^[a-f0-9]{64}$').hasMatch(csrf)) {
      throw Exception('映匣后台返回了无效连接');
    }
    if (_disposed) throw StateError('连接已关闭');
    _isAndroidEmulator = Platform.isAndroid && bootstrap['emulator'] == true;
    _origin = uri.toString();
    _cookie = cookie;
    _csrf = csrf;
    _client = HttpClient()..connectionTimeout = const Duration(seconds: 10);
    await get('/api/state');
  }

  Future<dynamic> get(String path) => _request('GET', path, null);
  Future<dynamic> post(String path, Map<String, dynamic> data) =>
      _request('POST', path, data);

  Future<dynamic> _request(
    String method,
    String path,
    Map<String, dynamic>? data,
  ) async {
    if (_disposed || _origin.isEmpty) throw StateError('映匣后台尚未连接');
    final base = Uri.parse(_origin);
    final normalized = path.startsWith('/api/') ? path : '/api/$path';
    if (path.contains('://') || path.startsWith('//')) {
      throw ArgumentError('仅允许访问映匣本机接口');
    }
    final uri = base.resolve(normalized);
    if (uri.origin != base.origin || !uri.path.startsWith('/api/')) {
      throw ArgumentError('仅允许访问映匣本机接口');
    }
    final request = await _client!
        .openUrl(method, uri)
        .timeout(const Duration(seconds: 15));
    request.headers.set('Cookie', _cookie);
    request.headers.set('Origin', _origin);
    if (data != null) {
      final bytes = utf8.encode(jsonEncode(data));
      if (bytes.length > 65536) throw ArgumentError('请求内容过大');
      request.headers.contentType = ContentType.json;
      request.headers.set('X-YingXia-CSRF', _csrf);
      request.contentLength = bytes.length;
      request.add(bytes);
    }
    final response = await request.close().timeout(
      const Duration(seconds: 160),
    );
    final body = await utf8.decoder
        .bind(response)
        .join()
        .timeout(const Duration(seconds: 160));
    final decoded = jsonDecode(body) as Map<String, dynamic>;
    if (decoded['ok'] != true) throw Exception(decoded['message'] ?? '请求未完成');
    return decoded['data'];
  }

  Future<void> dispose() async {
    if (_disposed) return;
    // Android shutdown must pass through the Activity ownership guard.
    if (_origin.isNotEmpty && !Platform.isAndroid) {
      try {
        await post('/api/quit', {}).timeout(const Duration(seconds: 3));
      } catch (_) {}
    }
    _disposed = true;
    _client?.close(force: true);
    if (Platform.isAndroid) {
      try {
        await _channel.invokeMethod<void>('stop');
      } catch (_) {}
    }
    final process = _process;
    if (process != null) {
      await process.stdin.close();
      try {
        await process.exitCode.timeout(const Duration(seconds: 7));
      } catch (_) {
        process.kill();
      }
    }
    await _stdout?.cancel();
    await _stderr?.cancel();
  }
}
