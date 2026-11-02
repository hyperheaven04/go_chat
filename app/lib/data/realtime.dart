import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'api_client.dart';
import 'models.dart';

enum ConnectionStatus { connecting, connected, disconnected }

sealed class RealtimeEvent {}

class MessageNewEvent extends RealtimeEvent {
  MessageNewEvent(this.message);
  final Message message;
}

class TypingEvent extends RealtimeEvent {
  TypingEvent(this.dialogId, this.userId, this.active);
  final String dialogId;
  final String userId;
  final bool active;
}

class MessageReadEvent extends RealtimeEvent {
  MessageReadEvent(this.dialogId, this.userId, this.messageId);
  final String dialogId;
  final String userId;
  final int messageId;
}

class PresenceEvent extends RealtimeEvent {
  PresenceEvent(this.userId, this.online, this.lastSeenAt);
  final String userId;
  final bool online;
  final DateTime? lastSeenAt;
}

class ServerErrorEvent extends RealtimeEvent {
  ServerErrorEvent(this.ref, this.code, this.message);
  final String? ref;
  final String code;
  final String message;
}

/// Keeps a WebSocket open while started, reconnecting with exponential
/// backoff. Every connection is authenticated with a fresh one-time ticket.
class RealtimeClient {
  RealtimeClient(this._api, this._wsBase);

  final ApiClient _api;
  final String _wsBase;

  final status = ValueNotifier(ConnectionStatus.disconnected);
  final _events = StreamController<RealtimeEvent>.broadcast();
  Stream<RealtimeEvent> get events => _events.stream;

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _sub;
  Timer? _retryTimer;
  int _attempt = 0;
  bool _running = false;

  void start() {
    if (_running) return;
    _running = true;
    _connect();
  }

  void stop() {
    _running = false;
    _retryTimer?.cancel();
    _sub?.cancel();
    _channel?.sink.close();
    _channel = null;
    status.value = ConnectionStatus.disconnected;
  }

  /// Returns false if not connected; callers keep the item pending and retry
  /// after reconnecting.
  bool send(String type, Map<String, dynamic> payload, {String? ref}) {
    final ch = _channel;
    if (ch == null || status.value != ConnectionStatus.connected) return false;
    ch.sink.add(jsonEncode({'type': type, 'ref': ?ref, 'payload': payload}));
    return true;
  }

  Future<void> _connect() async {
    if (!_running) return;
    status.value = ConnectionStatus.connecting;
    try {
      final ticket = await _api.wsTicket();
      final ch = WebSocketChannel.connect(
        Uri.parse(
          '$_wsBase/api/v1/ws',
        ).replace(queryParameters: {'ticket': ticket}),
      );
      await ch.ready;
      if (!_running) {
        ch.sink.close();
        return;
      }
      _channel = ch;
      _attempt = 0;
      _sub = ch.stream.listen(
        _onFrame,
        onDone: _onClosed,
        onError: (_) => _onClosed(),
      );
      status.value = ConnectionStatus.connected;
    } catch (_) {
      _onClosed();
    }
  }

  void _onClosed() {
    _sub?.cancel();
    _channel = null;
    if (!_running) return;
    status.value = ConnectionStatus.disconnected;
    // 1s, 2s, 4s ... capped at 30s, with jitter so clients don't reconnect in lockstep.
    final delay = min(30, pow(2, _attempt).toInt());
    _attempt++;
    _retryTimer = Timer(
      Duration(milliseconds: delay * 1000 + Random().nextInt(500)),
      _connect,
    );
  }

  void _onFrame(dynamic raw) {
    final Map<String, dynamic> env;
    try {
      env = jsonDecode(raw as String) as Map<String, dynamic>;
    } catch (_) {
      return;
    }
    final p = env['payload'] as Map<String, dynamic>? ?? const {};
    final event = switch (env['type']) {
      'message.new' => MessageNewEvent(Message.fromJson(p)),
      'typing' => TypingEvent(
        p['dialog_id'] as String,
        p['user_id'] as String,
        p['active'] as bool,
      ),
      'message.read' => MessageReadEvent(
        p['dialog_id'] as String,
        p['user_id'] as String,
        p['message_id'] as int,
      ),
      'presence' => PresenceEvent(
        p['user_id'] as String,
        p['online'] as bool,
        p['last_seen_at'] == null
            ? null
            : DateTime.parse(p['last_seen_at'] as String).toLocal(),
      ),
      'error' => ServerErrorEvent(
        env['ref'] as String?,
        p['code'] as String,
        p['message'] as String,
      ),
      _ => null,
    };
    if (event != null) _events.add(event);
  }
}
