import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'models.dart';

/// Persists the access token in the platform keystore (Keychain / Keystore),
/// not in plain shared preferences.
class SessionStore {
  static const _storage = FlutterSecureStorage();
  static const _tokenKey = 'token';
  static const _userKey = 'user';

  Future<void> save(String token, User user) async {
    await _storage.write(key: _tokenKey, value: token);
    await _storage.write(key: _userKey, value: jsonEncode(user.toJson()));
  }

  Future<(String, User)?> load() async {
    final token = await _storage.read(key: _tokenKey);
    final user = await _storage.read(key: _userKey);
    if (token == null || user == null) return null;
    return (token, User.fromJson(jsonDecode(user) as Map<String, dynamic>));
  }

  Future<void> clear() => _storage.deleteAll();
}
