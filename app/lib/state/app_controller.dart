import 'package:flutter/foundation.dart';

import '../config.dart';
import '../data/api_client.dart';
import '../data/models.dart';
import '../data/realtime.dart';
import '../data/session_store.dart';

/// Owns the session: API client, realtime connection and the current user.
class AppController extends ChangeNotifier {
  AppController() {
    api.onUnauthorized = logout;
  }

  final api = ApiClient(AppConfig.apiUrl);
  late final realtime = RealtimeClient(api, AppConfig.wsUrl);
  final _store = SessionStore();

  User? me;
  bool restoring = true;

  Future<void> restore() async {
    try {
      final saved = await _store.load();
      if (saved != null) {
        api.token = saved.$1;
        me = saved.$2;
        realtime.start();
        // Validate the token in the background; a 401 triggers logout.
        api.me().then((u) {
          me = u;
          notifyListeners();
        }, onError: (_) {});
      }
    } finally {
      restoring = false;
      notifyListeners();
    }
  }

  Future<void> login(String username, String password) async =>
      _start(await api.login(username, password));

  Future<void> register(String username, String password) async =>
      _start(await api.register(username, password));

  Future<void> _start(AuthResult res) async {
    api.token = res.token;
    me = res.user;
    await _store.save(res.token, res.user);
    realtime.start();
    notifyListeners();
  }

  Future<void> logout() async {
    if (me == null) return;
    realtime.stop();
    api.token = null;
    me = null;
    await _store.clear();
    notifyListeners();
  }
}
