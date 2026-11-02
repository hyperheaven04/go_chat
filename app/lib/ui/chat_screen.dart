import 'package:flutter/material.dart';

import '../data/models.dart';
import '../state/app_controller.dart';
import '../state/chat_controller.dart';
import 'format.dart';
import 'widgets.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key, required this.app, required this.dialog});

  final AppController app;
  final Conversation dialog;

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> with WidgetsBindingObserver {
  late final _ctrl = ChatController(widget.app, widget.dialog);
  final _input = TextEditingController();
  final _scroll = ScrollController();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _ctrl.addListener(_showErrors);
    _scroll.addListener(() {
      // The list is reversed: the top of the chat is maxScrollExtent.
      if (_scroll.position.pixels > _scroll.position.maxScrollExtent - 300) {
        _ctrl.loadMore();
      }
    });
  }

  // Messages count as read only while the user can actually see them.
  @override
  void didChangeAppLifecycleState(AppLifecycleState state) =>
      _ctrl.setForeground(state == AppLifecycleState.resumed);

  void _showErrors() {
    final e = _ctrl.error;
    if (e == null || !mounted) return;
    _ctrl.error = null;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e)));
  }

  void _send() {
    _ctrl.send(_input.text);
    _input.clear();
    if (_scroll.hasClients) {
      _scroll.animateTo(
        0,
        duration: const Duration(milliseconds: 200),
        curve: Curves.easeOut,
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return ListenableBuilder(
      listenable: _ctrl,
      builder: (context, _) {
        final peer = _ctrl.dialog.peer;
        return Scaffold(
          appBar: AppBar(
            titleSpacing: 0,
            title: Row(
              children: [
                UserAvatar(peer, radius: 18),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(peer.username, style: const TextStyle(fontSize: 17)),
                      AnimatedSwitcher(
                        duration: const Duration(milliseconds: 150),
                        child: Text(
                          _ctrl.peerTyping ? 'печатает…' : formatPresence(peer),
                          key: ValueKey(_ctrl.peerTyping),
                          style: TextStyle(
                            fontSize: 12,
                            color: _ctrl.peerTyping || peer.online
                                ? scheme.primary
                                : scheme.onSurfaceVariant,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            bottom: PreferredSize(
              preferredSize: const Size.fromHeight(0),
              child: ConnectionBanner(widget.app.realtime.status),
            ),
          ),
          body: Column(
            children: [
              Expanded(child: _buildMessages()),
              _buildInput(scheme),
            ],
          ),
        );
      },
    );
  }

  Widget _buildMessages() {
    if (_ctrl.loading) return const Center(child: CircularProgressIndicator());
    final msgs = _ctrl.messages;
    if (msgs.isEmpty) {
      return Center(
        child: Text(
          'Напишите первое сообщение 👋',
          style: TextStyle(
            color: Theme.of(context).colorScheme.onSurfaceVariant,
          ),
        ),
      );
    }
    return ListView.builder(
      controller: _scroll,
      reverse: true,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      itemCount: msgs.length + (_ctrl.loadingMore ? 1 : 0),
      itemBuilder: (context, i) {
        if (i == msgs.length) {
          return const Padding(
            padding: EdgeInsets.all(12),
            child: Center(child: CircularProgressIndicator(strokeWidth: 2)),
          );
        }
        final m = msgs[i];
        final older = i + 1 < msgs.length ? msgs[i + 1] : null;
        final newDay =
            older == null || !isSameDay(older.createdAt, m.createdAt);
        final mine = m.senderId == _ctrl.myId;
        return Column(
          children: [
            if (newDay) _DayChip(formatDay(m.createdAt)),
            _Bubble(
              message: m,
              mine: mine,
              read: _ctrl.isRead(m),
              tightTop: !newDay && older.senderId == m.senderId,
              onRetry: () => _ctrl.retry(m),
            ),
          ],
        );
      },
    );
  }

  Widget _buildInput(ColorScheme scheme) {
    return SafeArea(
      top: false,
      child: Container(
        padding: const EdgeInsets.fromLTRB(12, 8, 8, 8),
        decoration: BoxDecoration(
          color: scheme.surface,
          border: Border(top: BorderSide(color: scheme.outlineVariant)),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            Expanded(
              child: TextField(
                controller: _input,
                onChanged: _ctrl.onTextChanged,
                minLines: 1,
                maxLines: 5,
                maxLength: 4000,
                textCapitalization: TextCapitalization.sentences,
                buildCounter:
                    (
                      _, {
                      required currentLength,
                      required isFocused,
                      maxLength,
                    }) => null,
                decoration: InputDecoration(
                  hintText: 'Сообщение',
                  filled: true,
                  fillColor: scheme.surfaceContainerHighest,
                  contentPadding: const EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 10,
                  ),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(24),
                    borderSide: BorderSide.none,
                  ),
                ),
              ),
            ),
            const SizedBox(width: 4),
            ValueListenableBuilder(
              valueListenable: _input,
              builder: (context, v, _) => IconButton.filled(
                onPressed: v.text.trim().isEmpty ? null : _send,
                icon: const Icon(Icons.send_rounded),
              ),
            ),
          ],
        ),
      ),
    );
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _ctrl.removeListener(_showErrors);
    _ctrl.dispose();
    _input.dispose();
    _scroll.dispose();
    super.dispose();
  }
}

class _DayChip extends StatelessWidget {
  const _DayChip(this.label);

  final String label;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 12),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
        decoration: BoxDecoration(
          color: scheme.surfaceContainerHigh,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Text(
          label,
          style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant),
        ),
      ),
    );
  }
}

class _Bubble extends StatelessWidget {
  const _Bubble({
    required this.message,
    required this.mine,
    required this.read,
    required this.tightTop,
    required this.onRetry,
  });

  final Message message;
  final bool mine;
  final bool read;
  final bool tightTop;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final bg = mine ? scheme.primary : scheme.surfaceContainerHigh;
    final fg = mine ? scheme.onPrimary : scheme.onSurface;
    final meta = fg.withValues(alpha: 0.7);
    const r = Radius.circular(18);

    final bubble = Container(
      constraints: BoxConstraints(
        maxWidth: MediaQuery.sizeOf(context).width * 0.75,
      ),
      margin: EdgeInsets.only(top: tightTop ? 2 : 8),
      padding: const EdgeInsets.fromLTRB(12, 8, 10, 6),
      decoration: BoxDecoration(
        color: message.status == MessageStatus.failed
            ? scheme.errorContainer
            : bg,
        borderRadius: BorderRadius.only(
          topLeft: r,
          topRight: r,
          bottomLeft: mine ? r : const Radius.circular(4),
          bottomRight: mine ? const Radius.circular(4) : r,
        ),
      ),
      child: Wrap(
        alignment: WrapAlignment.end,
        crossAxisAlignment: WrapCrossAlignment.end,
        spacing: 8,
        children: [
          Text(message.body, style: TextStyle(color: fg, fontSize: 15.5)),
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              // Server time; pending messages show local time until confirmed.
              Text(
                formatTime(message.createdAt),
                style: TextStyle(fontSize: 11, color: meta),
              ),
              if (mine) ...[
                const SizedBox(width: 3),
                _statusIcon(meta, scheme),
              ],
            ],
          ),
        ],
      ),
    );

    return Align(
      alignment: mine ? Alignment.centerRight : Alignment.centerLeft,
      child: message.status == MessageStatus.failed
          ? GestureDetector(onTap: onRetry, child: bubble)
          : bubble,
    );
  }

  Widget _statusIcon(Color meta, ColorScheme scheme) =>
      switch (message.status) {
        MessageStatus.pending => Icon(Icons.schedule, size: 14, color: meta),
        MessageStatus.failed => Icon(
          Icons.error_outline,
          size: 14,
          color: scheme.error,
        ),
        MessageStatus.sent => Icon(
          read ? Icons.done_all : Icons.done,
          size: 15,
          color: meta,
        ),
      };
}
