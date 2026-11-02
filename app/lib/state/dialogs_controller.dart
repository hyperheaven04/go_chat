import 'dart:async';

import 'package:flutter/foundation.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import '../data/realtime.dart';
import 'app_controller.dart';

class DialogsController extends ChangeNotifier {
  DialogsController(this._app) {
    _sub = _app.realtime.events.listen(_onEvent);
    _app.realtime.status.addListener(_onStatus);
    load();
  }

  final AppController _app;
  late final StreamSubscription<RealtimeEvent> _sub;

  List<Conversation> dialogs = [];
  bool loading = true;
  String? error;

  /// The dialog currently open on screen: its messages don't count as unread.
  String? activeDialogId;

  Future<void> load() async {
    try {
      dialogs = await _app.api.dialogs();
      error = null;
    } on ApiException catch (e) {
      error = e.message;
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  void _onStatus() {
    // Anything may have happened while we were offline.
    if (_app.realtime.status.value == ConnectionStatus.connected) load();
  }

  void _onEvent(RealtimeEvent e) {
    final me = _app.me?.id;
    switch (e) {
      case MessageNewEvent(:final message):
        final i = dialogs.indexWhere((d) => d.id == message.dialogId);
        if (i < 0) {
          load(); // a new dialog started by someone else
          return;
        }
        final d = dialogs[i];
        final unread = message.senderId != me && activeDialogId != d.id;
        final updated = d.copyWith(
          lastMessage: message,
          unreadCount: d.unreadCount + (unread ? 1 : 0),
        );
        dialogs = [updated, ...dialogs.where((x) => x.id != d.id)];
      case MessageReadEvent(:final dialogId, :final userId, :final messageId):
        dialogs = [
          for (final d in dialogs)
            if (d.id != dialogId)
              d
            else if (userId == me)
              d.copyWith(
                myLastReadId: messageId,
                unreadCount: messageId >= (d.lastMessage?.id ?? 0)
                    ? 0
                    : d.unreadCount,
              )
            else
              d.copyWith(peerLastReadId: messageId),
        ];
      case PresenceEvent(:final userId, :final online, :final lastSeenAt):
        dialogs = [
          for (final d in dialogs)
            d.peer.id == userId
                ? d.copyWith(
                    peer: d.peer.copyWith(
                      online: online,
                      lastSeenAt: lastSeenAt,
                    ),
                  )
                : d,
        ];
      case TypingEvent() || ServerErrorEvent():
        return;
    }
    notifyListeners();
  }

  @override
  void dispose() {
    _sub.cancel();
    _app.realtime.status.removeListener(_onStatus);
    super.dispose();
  }
}
