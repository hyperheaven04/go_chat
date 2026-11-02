import 'package:flutter/material.dart';

import '../data/models.dart';
import '../state/app_controller.dart';
import '../state/dialogs_controller.dart';
import 'chat_screen.dart';
import 'format.dart';
import 'search_screen.dart';
import 'widgets.dart';

class DialogsScreen extends StatefulWidget {
  const DialogsScreen({super.key, required this.app});

  final AppController app;

  @override
  State<DialogsScreen> createState() => _DialogsScreenState();
}

class _DialogsScreenState extends State<DialogsScreen> {
  late final _ctrl = DialogsController(widget.app);

  Future<void> _openChat(Conversation d) async {
    _ctrl.activeDialogId = d.id;
    await Navigator.push(
      context,
      MaterialPageRoute(
        builder: (_) => ChatScreen(app: widget.app, dialog: d),
      ),
    );
    _ctrl.activeDialogId = null;
  }

  Future<void> _newChat() async {
    final d = await Navigator.push<Conversation>(
      context,
      MaterialPageRoute(builder: (_) => SearchScreen(app: widget.app)),
    );
    if (d == null) return;
    await _ctrl.load();
    if (mounted) _openChat(d);
  }

  @override
  Widget build(BuildContext context) {
    final me = widget.app.me!;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Чаты'),
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: 4),
            child: Center(child: Text(me.username)),
          ),
          IconButton(
            tooltip: 'Выйти',
            icon: const Icon(Icons.logout),
            onPressed: widget.app.logout,
          ),
        ],
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(0),
          child: ConnectionBanner(widget.app.realtime.status),
        ),
      ),
      floatingActionButton: FloatingActionButton(
        tooltip: 'Новый чат',
        onPressed: _newChat,
        child: const Icon(Icons.edit_outlined),
      ),
      body: ListenableBuilder(
        listenable: _ctrl,
        builder: (context, _) {
          if (_ctrl.loading) {
            return const Center(child: CircularProgressIndicator());
          }
          if (_ctrl.error != null && _ctrl.dialogs.isEmpty) {
            return _Placeholder(
              icon: Icons.cloud_off,
              text: _ctrl.error!,
              action: TextButton(
                onPressed: _ctrl.load,
                child: const Text('Повторить'),
              ),
            );
          }
          if (_ctrl.dialogs.isEmpty) {
            return _Placeholder(
              icon: Icons.chat_bubble_outline,
              text: 'Пока нет диалогов',
              action: FilledButton.tonal(
                onPressed: _newChat,
                child: const Text('Найти собеседника'),
              ),
            );
          }
          return RefreshIndicator(
            onRefresh: _ctrl.load,
            child: ListView.separated(
              itemCount: _ctrl.dialogs.length,
              separatorBuilder: (_, _) => const Divider(height: 1, indent: 76),
              itemBuilder: (context, i) => _DialogTile(
                dialog: _ctrl.dialogs[i],
                myId: me.id,
                onTap: _openChat,
              ),
            ),
          );
        },
      ),
    );
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }
}

class _DialogTile extends StatelessWidget {
  const _DialogTile({
    required this.dialog,
    required this.myId,
    required this.onTap,
  });

  final Conversation dialog;
  final String myId;
  final void Function(Conversation) onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final last = dialog.lastMessage;
    final mine = last != null && last.senderId == myId;
    return ListTile(
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      leading: UserAvatar(dialog.peer, radius: 26),
      title: Row(
        children: [
          Expanded(
            child: Text(
              dialog.peer.username,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
          ),
          if (mine)
            Padding(
              padding: const EdgeInsets.only(right: 4),
              child: Icon(
                last.id! <= dialog.peerLastReadId ? Icons.done_all : Icons.done,
                size: 16,
                color: scheme.primary,
              ),
            ),
          if (last != null)
            Text(
              formatDialogTime(last.createdAt),
              style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant),
            ),
        ],
      ),
      subtitle: Row(
        children: [
          Expanded(
            child: Text(
              last == null ? 'Нет сообщений' : (mine ? 'Вы: ' : '') + last.body,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
          if (dialog.unreadCount > 0)
            Badge(
              label: Text('${dialog.unreadCount}'),
              backgroundColor: scheme.primary,
              textColor: scheme.onPrimary,
            ),
        ],
      ),
      onTap: () => onTap(dialog),
    );
  }
}

class _Placeholder extends StatelessWidget {
  const _Placeholder({
    required this.icon,
    required this.text,
    required this.action,
  });

  final IconData icon;
  final String text;
  final Widget action;

  @override
  Widget build(BuildContext context) {
    final color = Theme.of(context).colorScheme.onSurfaceVariant;
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 56, color: color),
          const SizedBox(height: 12),
          Text(text, style: TextStyle(color: color)),
          const SizedBox(height: 16),
          action,
        ],
      ),
    );
  }
}
