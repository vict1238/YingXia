// SPDX-License-Identifier: GPL-3.0-or-later
import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';

import 'backend.dart';

typedef SourceJson = Map<String, dynamic>;
SourceJson _map(dynamic value) =>
    value is Map ? Map<String, dynamic>.from(value) : <String, dynamic>{};
List<SourceJson> _rows(dynamic value) =>
    value is List
        ? value
            .whereType<Map>()
            .map((e) => Map<String, dynamic>.from(e))
            .toList()
        : [];
String _text(dynamic value) => value?.toString() ?? '';

/// Bounded source list and an asynchronous connection-latency job. Its state is
/// local to this page; leaving the page cancels outstanding measurements.
class SourceManager extends StatefulWidget {
  const SourceManager({
    super.key,
    required this.backend,
    required this.registry,
    required this.ensureNetwork,
    required this.refresh,
    required this.onRegistry,
    required this.toggleProvider,
    required this.saveAutoUpdate,
  });
  final BackendConnection backend;
  final SourceJson registry;
  final Future<bool> Function() ensureNetwork;
  final Future<void> Function() refresh;
  final ValueChanged<SourceJson> onRegistry;
  final Future<void> Function(SourceJson source, bool enabled) toggleProvider;
  final Future<void> Function(bool enabled) saveAutoUpdate;

  @override
  State<SourceManager> createState() => _SourceManagerState();
}

class _SourceManagerState extends State<SourceManager> {
  static const _pageSize = 15;
  final _query = TextEditingController();
  SourceJson? _measuredRegistry;
  SourceJson? _job;
  Timer? _poll;
  int _generation = 0;
  int _page = 0;
  String _filter = 'all';
  String _error = '';
  bool _starting = false;
  bool _updating = false;
  final Set<String> _busy = {};

  SourceJson get _registry => _measuredRegistry ?? widget.registry;
  List<SourceJson> get _providers => _rows(_registry['providers']);
  bool get _running => _starting || _job?['state'] == 'running';

  @override
  void didUpdateWidget(covariant SourceManager oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.registry, widget.registry)) {
      _measuredRegistry = null;
    }
  }

  @override
  void dispose() {
    _generation++;
    _poll?.cancel();
    final id = _text(_job?['id']);
    if (_job?['state'] == 'running' && id.isNotEmpty) {
      unawaited(
        widget.backend
            .post('sources/speed/cancel', {'id': id})
            .catchError((_) => null),
      );
    }
    _query.dispose();
    super.dispose();
  }

  void _receive(SourceJson job) {
    if (!mounted) return;
    final registry = _map(job['sources']);
    setState(() {
      _job = job;
      if (registry['providers'] is List) _measuredRegistry = registry;
    });
    if (registry['providers'] is List) widget.onRegistry(registry);
  }

  Future<void> _measure([String? sourceId]) async {
    if (_running) return;
    final generation = ++_generation;
    setState(() {
      _starting = true;
      _error = '';
      _job = null;
    });
    try {
      if (!await widget.ensureNetwork() ||
          !mounted ||
          generation != _generation) {
        return;
      }
      final job = _map(
        await widget.backend.post('sources/speed/start', {
          if (sourceId != null) 'ids': [sourceId],
        }),
      );
      if (!mounted || generation != _generation) {
        final id = _text(job['id']);
        if (id.isNotEmpty && job['state'] == 'running') {
          unawaited(
            widget.backend
                .post('sources/speed/cancel', {'id': id})
                .catchError((_) => null),
          );
        }
        return;
      }
      if (_text(job['id']).isEmpty) throw StateError('测速未能启动');
      _receive(job);
      if (job['state'] == 'running') _schedulePoll(generation);
    } catch (e) {
      if (mounted && generation == _generation) {
        setState(() => _error = e.toString());
      }
    } finally {
      if (mounted && generation == _generation) {
        setState(() => _starting = false);
      }
    }
  }

  void _schedulePoll(int generation) {
    _poll?.cancel();
    _poll = Timer(const Duration(milliseconds: 500), () async {
      try {
        final id = Uri.encodeQueryComponent(_text(_job?['id']));
        final job = _map(
          await widget.backend.get('sources/speed/status?id=$id'),
        );
        if (!mounted || generation != _generation) return;
        _receive(job);
        if (job['state'] == 'running') _schedulePoll(generation);
      } catch (e) {
        if (mounted && generation == _generation) {
          setState(() => _error = e.toString());
          // A transient status failure must not enable a second concurrent job.
          _schedulePoll(generation);
        }
      }
    });
  }

  Future<void> _cancel() async {
    final id = _text(_job?['id']);
    if (id.isEmpty) return;
    try {
      final job = _map(
        await widget.backend.post('sources/speed/cancel', {'id': id}),
      );
      if (!mounted) return;
      _poll?.cancel();
      _generation++;
      _receive({..._job!, ...job, 'state': 'cancelled'});
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    }
  }

  Future<void> _update() async {
    if (_updating || !await widget.ensureNetwork() || !mounted) return;
    setState(() {
      _updating = true;
      _error = '';
    });
    try {
      await widget.backend.post('sources/update', {});
      await widget.refresh();
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _updating = false);
    }
  }

  Future<void> _edit([SourceJson? source]) async {
    final patch = await showDialog<SourceJson>(
      context: context,
      builder: (_) => _SourceDialog(source: source),
    );
    if (patch == null || !mounted) return;
    try {
      await widget.backend.post(
        source == null ? 'sources/add' : 'sources/edit',
        {if (source != null) 'id': source['id'], ...patch},
      );
      await widget.refresh();
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    }
  }

  Future<void> _remove(SourceJson source) async {
    final accepted = await showDialog<bool>(
      context: context,
      builder:
          (context) => AlertDialog(
            title: Text('删除“${source['name']}”？'),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(context, false),
                child: const Text('取消'),
              ),
              FilledButton(
                onPressed: () => Navigator.pop(context, true),
                child: const Text('删除'),
              ),
            ],
          ),
    );
    if (accepted != true || !mounted) return;
    try {
      await widget.backend.post('sources/remove', {'id': source['id']});
      await widget.refresh();
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    }
  }

  Future<void> _toggle(SourceJson source, bool enabled) async {
    final id = _text(source['id']);
    setState(() => _busy.add(id));
    try {
      await widget.toggleProvider(source, enabled);
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _busy.remove(id));
    }
  }

  List<SourceJson> get _filtered {
    final query = _query.text.trim().toLowerCase();
    return _providers.where((source) {
      final status = _text(_map(source['health'])['status']);
      final matches =
          _filter == 'all' ||
          (_filter == 'enabled' && source['enabled'] == true) ||
          (_filter == 'unknown' && (status.isEmpty || status == 'unknown')) ||
          _filter == status;
      return matches &&
          (query.isEmpty ||
              '${source['name']} ${source['kind']} ${source['id']}'
                  .toLowerCase()
                  .contains(query));
    }).toList();
  }

  @override
  Widget build(BuildContext context) {
    final filtered = _filtered;
    final pages = (filtered.length / _pageSize).ceil().clamp(1, 100000);
    final page = _page.clamp(0, pages - 1);
    final visible = filtered.skip(page * _pageSize).take(_pageSize).toList();
    final completed = (_job?['completed'] as num? ?? 0).toInt();
    final total = (_job?['total'] as num? ?? 0).toInt();
    return Container(
      key: const Key('source-manager'),
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: const Color(0xff182126),
        border: Border.all(color: const Color(0xff2b393f)),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            alignment: WrapAlignment.spaceBetween,
            crossAxisAlignment: WrapCrossAlignment.center,
            spacing: 16,
            runSpacing: 10,
            children: [
              Text(
                '来源管理（${_providers.length}）',
                style: const TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w600,
                ),
              ),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  OutlinedButton(
                    key: const Key('source-update'),
                    onPressed: _updating || _running ? null : _update,
                    child: Text(_updating ? '更新中…' : '更新来源'),
                  ),
                  OutlinedButton(
                    key: const Key('source-add'),
                    onPressed: _running ? null : _edit,
                    child: const Text('添加来源'),
                  ),
                  FilledButton(
                    key: const Key('source-measure-all'),
                    onPressed: _running || _updating ? null : _measure,
                    child: const Text('全部测速'),
                  ),
                ],
              ),
            ],
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              const Expanded(
                child: Text('自动更新来源', style: TextStyle(fontSize: 13)),
              ),
              Switch(
                key: const Key('source-auto-update'),
                value: _registry['auto_update'] == true,
                onChanged: widget.saveAutoUpdate,
              ),
            ],
          ),
          if (_text(_registry['last_checked_at']).isNotEmpty)
            Text(
              '上次更新检查：${_localTime(_registry['last_checked_at'])}',
              style: const TextStyle(fontSize: 11, color: Color(0xff98abaa)),
            ),
          if (_map(_registry['upstreams']).isNotEmpty)
            ExpansionTile(
              tilePadding: EdgeInsets.zero,
              childrenPadding: const EdgeInsets.only(bottom: 10),
              title: const Text('更新详情', style: TextStyle(fontSize: 12)),
              children: [
                for (final entry in _map(_registry['upstreams']).entries)
                  Align(
                    alignment: Alignment.centerLeft,
                    child: Padding(
                      padding: const EdgeInsets.symmetric(vertical: 4),
                      child: Text(
                        [
                          {
                                'kazumi_rules': 'Kazumi 规则',
                                'kazumi': 'Kazumi',
                                '88lin': '88lin',
                              }[entry.key] ??
                              entry.key,
                          _map(entry.value)['version'],
                          _map(entry.value)['status'],
                          _map(entry.value)['error'],
                        ].map(_text).where((e) => e.isNotEmpty).join(' · '),
                        style: const TextStyle(
                          fontSize: 12,
                          color: Color(0xff98abaa),
                        ),
                      ),
                    ),
                  ),
                if ((_registry['changes'] as List? ?? []).isNotEmpty)
                  Text(
                    (_registry['changes'] as List).map(_text).join('；'),
                    style: const TextStyle(fontSize: 12),
                  ),
              ],
            ),
          const SizedBox(height: 14),
          LayoutBuilder(
            builder:
                (_, constraints) => Wrap(
                  spacing: 10,
                  runSpacing: 10,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    SizedBox(
                      width:
                          constraints.maxWidth < 400
                              ? constraints.maxWidth
                              : 245,
                      child: TextField(
                        key: const Key('source-filter-query'),
                        controller: _query,
                        decoration: const InputDecoration(
                          hintText: '查找来源',
                          prefixIcon: Icon(Icons.search, size: 18),
                        ),
                        onChanged: (_) => setState(() => _page = 0),
                      ),
                    ),
                    SizedBox(
                      width: 130,
                      child: DropdownButtonFormField<String>(
                        key: const Key('source-filter'),
                        initialValue: _filter,
                        isExpanded: true,
                        decoration: const InputDecoration(
                          contentPadding: EdgeInsets.symmetric(
                            horizontal: 12,
                            vertical: 11,
                          ),
                        ),
                        items:
                            const {
                                  'all': '全部',
                                  'enabled': '已启用',
                                  'available': '已响应',
                                  'unavailable': '未响应',
                                  'unknown': '待测速',
                                }.entries
                                .map(
                                  (e) => DropdownMenuItem(
                                    value: e.key,
                                    child: Text(
                                      e.value,
                                      style: const TextStyle(fontSize: 12),
                                    ),
                                  ),
                                )
                                .toList(),
                        onChanged:
                            (value) => setState(() {
                              _filter = value ?? 'all';
                              _page = 0;
                            }),
                      ),
                    ),
                  ],
                ),
          ),
          if (_job != null || _starting) ...[
            const SizedBox(height: 16),
            LinearProgressIndicator(
              key: const Key('source-speed-progress'),
              value: total == 0 ? null : (completed / total).clamp(0, 1),
            ),
            const SizedBox(height: 7),
            Row(
              children: [
                Expanded(
                  child: Text(
                    '${_running
                        ? '测速中'
                        : _job?['state'] == 'cancelled'
                        ? '已取消'
                        : _job?['state'] == 'failed'
                        ? '测速失败'
                        : '测速完成'} · $completed / $total',
                    style: const TextStyle(
                      fontSize: 12,
                      color: Color(0xff98abaa),
                    ),
                  ),
                ),
                if (_running && _job != null)
                  TextButton(
                    key: const Key('source-speed-cancel'),
                    onPressed: _cancel,
                    child: const Text('取消测速'),
                  ),
              ],
            ),
          ],
          if (_error.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: Text(
                _error,
                style: const TextStyle(fontSize: 12, color: Color(0xfff0aaa3)),
              ),
            ),
          const SizedBox(height: 14),
          if (visible.isEmpty)
            const Padding(padding: EdgeInsets.all(18), child: Text('没有匹配的来源')),
          for (final source in visible) _sourceRow(source),
          if (pages > 1)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: Wrap(
                alignment: WrapAlignment.end,
                crossAxisAlignment: WrapCrossAlignment.center,
                children: [
                  Text(
                    '${page + 1} / $pages 页 · ${filtered.length} 个',
                    style: const TextStyle(
                      fontSize: 12,
                      color: Color(0xff98abaa),
                    ),
                  ),
                  IconButton(
                    key: const Key('source-page-previous'),
                    tooltip: '上一页',
                    onPressed:
                        page == 0
                            ? null
                            : () => setState(() => _page = page - 1),
                    icon: const Icon(Icons.chevron_left),
                  ),
                  IconButton(
                    key: const Key('source-page-next'),
                    tooltip: '下一页',
                    onPressed:
                        page + 1 == pages
                            ? null
                            : () => setState(() => _page = page + 1),
                    icon: const Icon(Icons.chevron_right),
                  ),
                ],
              ),
            ),
        ],
      ),
    );
  }

  Widget _sourceRow(SourceJson source) {
    final id = _text(source['id']);
    final health = _map(source['health']);
    final status = _text(health['status']);
    final available = status == 'available';
    final latency = health['latency_ms'] as num?;
    final custom =
        source['editable'] == true ||
        source['custom'] == true ||
        source['origin'] == 'custom';
    final summary = [
      source['kind'] == 'kazumi'
          ? '动漫'
          : source['kind'] == 'cms'
          ? 'CMS'
          : '影视',
      available
          ? '已响应'
          : status == 'unavailable'
          ? '未响应'
          : '待测速',
      if (available && latency != null) '${latency.round()} ms',
    ].join(' · ');
    final controls = Wrap(
      crossAxisAlignment: WrapCrossAlignment.center,
      spacing: 4,
      children: [
        Tooltip(
          message: source['enabled'] == true ? '停用' : '启用',
          child: Checkbox(
            key: Key('source-enabled-$id'),
            value: source['enabled'] == true,
            onChanged:
                _busy.contains(id) || _running
                    ? null
                    : (value) => _toggle(source, value == true),
          ),
        ),
        TextButton(
          key: Key('source-measure-$id'),
          onPressed: _running || _updating ? null : () => _measure(id),
          child: const Text('测速'),
        ),
        if (custom)
          PopupMenuButton<String>(
            key: Key('source-actions-$id'),
            tooltip: '管理来源',
            enabled: !_running,
            onSelected: (action) {
              if (action == 'edit') {
                unawaited(_edit(source));
              } else {
                unawaited(_remove(source));
              }
            },
            itemBuilder:
                (_) => const [
                  PopupMenuItem(value: 'edit', child: Text('编辑')),
                  PopupMenuItem(value: 'remove', child: Text('删除')),
                ],
          ),
      ],
    );
    final label = Tooltip(
      message: [
        source['name'],
        source['endpoint'] ?? source['url'],
        health['message'],
        if (_text(health['checked_at']).isNotEmpty)
          _localTime(health['checked_at']),
      ].map(_text).where((e) => e.isNotEmpty).join('\n'),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            _text(source['name']),
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 13),
          ),
          const SizedBox(height: 4),
          Text(
            summary,
            style: TextStyle(
              fontSize: 11,
              color:
                  available
                      ? const Color(0xff89ddbd)
                      : status == 'unavailable'
                      ? const Color(0xfff0aaa3)
                      : const Color(0xff98abaa),
            ),
          ),
        ],
      ),
    );
    return Container(
      key: Key('source-row-$id'),
      padding: const EdgeInsets.symmetric(vertical: 11),
      decoration: const BoxDecoration(
        border: Border(top: BorderSide(color: Color(0xff2b393f))),
      ),
      child: LayoutBuilder(
        builder:
            (_, constraints) =>
                constraints.maxWidth < 430
                    ? Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        label,
                        Align(
                          alignment: Alignment.centerRight,
                          child: controls,
                        ),
                      ],
                    )
                    : Row(
                      children: [
                        Expanded(child: label),
                        const SizedBox(width: 12),
                        controls,
                      ],
                    ),
      ),
    );
  }
}

String _localTime(dynamic raw) {
  final date = DateTime.tryParse(_text(raw))?.toLocal();
  if (date == null) return _text(raw);
  String two(int value) => value.toString().padLeft(2, '0');
  return '${date.month}-${two(date.day)} ${two(date.hour)}:${two(date.minute)}';
}

class _SourceDialog extends StatefulWidget {
  const _SourceDialog({this.source});
  final SourceJson? source;
  @override
  State<_SourceDialog> createState() => _SourceDialogState();
}

class _SourceDialogState extends State<_SourceDialog> {
  late final _name = TextEditingController(text: _text(widget.source?['name']));
  // Public source URLs may be redacted. An empty edit keeps the original URL.
  final _url = TextEditingController();
  final _json = TextEditingController();
  String _error = '';
  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    _json.dispose();
    super.dispose();
  }

  void _submit() {
    try {
      SourceJson data;
      if (widget.source == null && _json.text.trim().isNotEmpty) {
        final parsed = jsonDecode(_json.text);
        final items = parsed is List ? parsed : _map(parsed)['sources'];
        if (items is! List || items.isEmpty) {
          throw const FormatException('请输入来源 JSON 数组');
        }
        data = {'items': items};
      } else {
        if (_name.text.trim().isEmpty) throw const FormatException('请输入名称');
        final address = _url.text.trim();
        final uri = Uri.tryParse(address);
        if ((widget.source == null || address.isNotEmpty) &&
            (uri == null ||
                !uri.hasAuthority ||
                !['http', 'https'].contains(uri.scheme))) {
          throw const FormatException('请输入正确的接口地址');
        }
        data = {'name': _name.text.trim(), 'url': _url.text.trim()};
      }
      Navigator.pop(context, data);
    } catch (e) {
      setState(
        () => _error = e.toString().replaceFirst('FormatException: ', ''),
      );
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: Text(widget.source == null ? '添加来源' : '编辑来源'),
    content: SizedBox(
      width: 480,
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(
              key: const Key('source-dialog-name'),
              controller: _name,
              maxLength: 80,
              decoration: const InputDecoration(
                labelText: '名称',
                hintText: '来源名称',
                counterText: '',
              ),
            ),
            const SizedBox(height: 18),
            TextField(
              key: const Key('source-dialog-url'),
              controller: _url,
              decoration: InputDecoration(
                labelText: 'CMS 接口地址',
                hintText:
                    widget.source == null
                        ? 'https://…/api.php/provide/vod/'
                        : '留空保留当前地址',
              ),
            ),
            if (widget.source == null)
              ExpansionTile(
                tilePadding: EdgeInsets.zero,
                title: const Text('批量导入 JSON', style: TextStyle(fontSize: 12)),
                children: [
                  TextField(
                    controller: _json,
                    minLines: 4,
                    maxLines: 8,
                    decoration: const InputDecoration(
                      hintText:
                          '{"sources":[{"name":"来源名称","url":"https://…"}]}',
                    ),
                  ),
                ],
              ),
            if (_error.isNotEmpty)
              Text(
                _error,
                style: const TextStyle(color: Color(0xfff0aaa3), fontSize: 12),
              ),
          ],
        ),
      ),
    ),
    actions: [
      OutlinedButton(
        onPressed: () => Navigator.pop(context),
        child: const Text('取消'),
      ),
      FilledButton(
        key: const Key('source-dialog-submit'),
        onPressed: _submit,
        child: Text(widget.source == null ? '添加' : '保存'),
      ),
    ],
  );
}
