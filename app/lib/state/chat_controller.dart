import 'dart:async';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:uuid/uuid.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import '../data/realtime.dart';
import 'app_controller.dart';

class ChatController extends ChangeNotifier {
  ChatController(this._app, this.dialog) {
    _sub = _app.realtime.events.listen(_onEvent);
    _app.realtime.status.addListener(_onStatus);
    _loadLatest();
  }

  static const _typingRefresh = Duration(seconds: 3);
  // Longer than the sender's refresh interval: tolerates one lost event,
  // yet the indicator never sticks if "stop" is lost.
  static const _typingTimeout = Duration(seconds: 6);

  final AppController _app;
  late final StreamSubscription<RealtimeEvent> _sub;

  Conversation dialog;

  /// Newest first (the list view is reversed).
  List<Message> messages = [];
  bool loading = true;
  bool loadingMore = false;
  bool hasMore = true;
  bool peerTyping = false;
  String? error;

  Timer? _peerTypingTimer;
  DateTime? _lastTypingSent;
  bool _foreground = true;

  String get myId => _app.me!.id;

  bool isRead(Message m) => m.id != null && m.id! <= dialog.peerLastReadId;

  // --- loading ---------------------------------------------------------------

  Future<void> _loadLatest() async {
    try {
      final latest = await _app.api.messages(dialog.id);
      final pending = messages.where((m) => m.id == null);
      final latestIds = latest.map((m) => m.clientId).toSet();
      // Merge: keep older pages we already had and unsent local messages.
      final older = messages.where(
        (m) =>
            m.id != null &&
            !latestIds.contains(m.clientId) &&
            (latest.isEmpty || m.id! < latest.last.id!),
      );
      messages = [...pending, ...latest, ...older];
      if (messages.length == latest.length) hasMore = latest.length >= 50;
      dialog = await _app.api.dialog(dialog.id);
      error = null;
      _markRead();
    } on ApiException catch (e) {
      error = e.message;
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  Future<void> loadMore() async {
    if (loadingMore || !hasMore || loading) return;
    final oldest = messages.lastWhere(
      (m) => m.id != null,
      orElse: () => messages.last,
    );
    if (oldest.id == null) return;
    loadingMore = true;
    notifyListeners();
    try {
      final page = await _app.api.messages(dialog.id, beforeId: oldest.id);
      messages = [...messages, ...page];
      hasMore = page.length >= 50;
    } on ApiException catch (e) {
      error = e.message;
    } finally {
      loadingMore = false;
      notifyListeners();
    }
  }

  // --- sending ---------------------------------------------------------------

  void send(String text) {
    final body = text.trim();
    if (body.isEmpty) return;
    final m = Message(
      id: null,
      clientId: const Uuid().v4(),
      dialogId: dialog.id,
      senderId: myId,
      body: body,
      createdAt: DateTime.now(),
      status: MessageStatus.pending,
    );
    messages = [m, ...messages];
    notifyListeners();
    _transmit(m);
    _sendTyping(false);
  }

  void retry(Message m) {
    _replace(m.clientId, m.copyWith(status: MessageStatus.pending));
    _transmit(m);
  }

  /// Offline sends stay pending and go out after reconnect; the server
  /// deduplicates by client_id, so resending is always safe.
  void _transmit(Message m) {
    _app.realtime.send('message.send', {
      'dialog_id': m.dialogId,
      'client_id': m.clientId,
      'body': m.body,
    }, ref: m.clientId);
  }

  void onTextChanged(String text) {
    if (text.trim().isEmpty) {
      if (_lastTypingSent != null) _sendTyping(false);
      return;
    }
    final now = DateTime.now();
    if (_lastTypingSent == null ||
        now.difference(_lastTypingSent!) > _typingRefresh) {
      _sendTyping(true);
    }
  }

  void _sendTyping(bool active) {
    _lastTypingSent = active ? DateTime.now() : null;
    _app.realtime.send('typing', {'dialog_id': dialog.id, 'active': active});
  }

  void setForeground(bool value) {
    _foreground = value;
    if (value) _markRead();
  }

  void _markRead() {
    if (!_foreground) return;
    final lastFromPeer = messages
        .where((m) => m.id != null && m.senderId != myId)
        .fold<int>(0, (acc, m) => max(acc, m.id!));
    if (lastFromPeer > dialog.myLastReadId) {
      final sent = _app.realtime.send('message.read', {
        'dialog_id': dialog.id,
        'message_id': lastFromPeer,
      });
      if (sent) dialog = dialog.copyWith(myLastReadId: lastFromPeer);
    }
  }

  // --- realtime --------------------------------------------------------------

  void _onStatus() {
    if (_app.realtime.status.value != ConnectionStatus.connected) return;
    for (final m in messages.where((m) => m.id == null)) {
      _transmit(m);
    }
    _loadLatest();
  }

  void _onEvent(RealtimeEvent e) {
    switch (e) {
      case MessageNewEvent(:final message) when message.dialogId == dialog.id:
        final i = messages.indexWhere((m) => m.clientId == message.clientId);
        if (i >= 0) {
          _replace(message.clientId, message); // our pending message confirmed
        } else {
          messages = [message, ...messages]..sort(_newestFirst);
        }
        if (message.senderId != myId) {
          _setPeerTyping(false);
          _markRead();
        }
      case TypingEvent(:final dialogId, :final active)
          when dialogId == dialog.id:
        _setPeerTyping(active);
      case MessageReadEvent(:final dialogId, :final userId, :final messageId)
          when dialogId == dialog.id:
        dialog = userId == myId
            ? dialog.copyWith(myLastReadId: max(dialog.myLastReadId, messageId))
            : dialog.copyWith(
                peerLastReadId: max(dialog.peerLastReadId, messageId),
              );
      case PresenceEvent(:final userId, :final online, :final lastSeenAt)
          when userId == dialog.peer.id:
        dialog = dialog.copyWith(
          peer: dialog.peer.copyWith(online: online, lastSeenAt: lastSeenAt),
        );
        if (!online) _setPeerTyping(false);
      case ServerErrorEvent(:final ref?, :final message):
        final i = messages.indexWhere((m) => m.clientId == ref);
        if (i < 0) return;
        _replace(ref, messages[i].copyWith(status: MessageStatus.failed));
        error = message;
      default:
        return;
    }
    notifyListeners();
  }

  void _setPeerTyping(bool active) {
    _peerTypingTimer?.cancel();
    peerTyping = active;
    if (active) {
      _peerTypingTimer = Timer(_typingTimeout, () {
        peerTyping = false;
        notifyListeners();
      });
    }
  }

  void _replace(String clientId, Message m) {
    messages = [for (final x in messages) x.clientId == clientId ? m : x];
    notifyListeners();
  }

  static int _newestFirst(Message a, Message b) {
    if (a.id == null) {
      return -1; // pending messages stay at the bottom of the chat
    }
    if (b.id == null) return 1;
    return b.id!.compareTo(a.id!);
  }

  @override
  void dispose() {
    if (_lastTypingSent != null) _sendTyping(false);
    _peerTypingTimer?.cancel();
    _sub.cancel();
    _app.realtime.status.removeListener(_onStatus);
    super.dispose();
  }
}
