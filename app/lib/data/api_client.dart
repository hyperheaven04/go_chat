import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import 'models.dart';

class ApiException implements Exception {
  ApiException(this.status, this.code, this.message, {this.field});

  final int status;
  final String code;
  final String message;
  final String? field;

  @override
  String toString() => message;
}

class AuthResult {
  AuthResult(this.user, this.token);
  final User user;
  final String token;
}

/// Thin typed wrapper over the REST API.
class ApiClient {
  ApiClient(this.baseUrl, {http.Client? client})
    : _http = client ?? http.Client();

  final String baseUrl;
  final http.Client _http;

  String? token;

  /// Called on any 401 from an authenticated request (expired/revoked token).
  void Function()? onUnauthorized;

  Future<AuthResult> register(String username, String password) =>
      _auth('/api/v1/auth/register', username, password);

  Future<AuthResult> login(String username, String password) =>
      _auth('/api/v1/auth/login', username, password);

  Future<AuthResult> _auth(
    String path,
    String username,
    String password,
  ) async {
    final j =
        await _request(
              'POST',
              path,
              body: {'username': username, 'password': password},
            )
            as Map<String, dynamic>;
    return AuthResult(
      User.fromJson(j['user'] as Map<String, dynamic>),
      j['token'] as String,
    );
  }

  Future<User> me() async => User.fromJson(
    await _request('GET', '/api/v1/users/me') as Map<String, dynamic>,
  );

  Future<List<User>> searchUsers(String query) async {
    final j = await _request('GET', '/api/v1/users', query: {'q': query});
    return [
      for (final u in j['users'] as List)
        User.fromJson(u as Map<String, dynamic>),
    ];
  }

  Future<List<Conversation>> dialogs() async {
    final j = await _request('GET', '/api/v1/dialogs');
    return [
      for (final d in j['dialogs'] as List)
        Conversation.fromJson(d as Map<String, dynamic>),
    ];
  }

  Future<Conversation> openDialog(String peerId) async => Conversation.fromJson(
    await _request('POST', '/api/v1/dialogs', body: {'peer_id': peerId})
        as Map<String, dynamic>,
  );

  Future<Conversation> dialog(String id) async => Conversation.fromJson(
    await _request('GET', '/api/v1/dialogs/$id') as Map<String, dynamic>,
  );

  /// Newest first. Pass [beforeId] to load older pages.
  Future<List<Message>> messages(
    String dialogId, {
    int? beforeId,
    int limit = 50,
  }) async {
    final j = await _request(
      'GET',
      '/api/v1/dialogs/$dialogId/messages',
      query: {
        if (beforeId != null) 'before_id': '$beforeId',
        'limit': '$limit',
      },
    );
    return [
      for (final m in j['messages'] as List)
        Message.fromJson(m as Map<String, dynamic>),
    ];
  }

  Future<String> wsTicket() async =>
      (await _request('POST', '/api/v1/ws/ticket'))['ticket'] as String;

  Future<dynamic> _request(
    String method,
    String path, {
    Object? body,
    Map<String, String>? query,
  }) async {
    final uri = Uri.parse('$baseUrl$path').replace(queryParameters: query);
    final req = http.Request(method, uri)
      ..headers['Content-Type'] = 'application/json'
      ..headers['Accept'] = 'application/json';
    if (token != null) req.headers['Authorization'] = 'Bearer $token';
    if (body != null) req.body = jsonEncode(body);

    final http.Response res;
    try {
      res = await http.Response.fromStream(
        await _http.send(req).timeout(const Duration(seconds: 15)),
      );
    } on TimeoutException {
      throw ApiException(0, 'timeout', 'Сервер не отвечает');
    } catch (_) {
      throw ApiException(0, 'network', 'Нет соединения с сервером');
    }

    final decoded = res.body.isEmpty
        ? null
        : jsonDecode(utf8.decode(res.bodyBytes));
    if (res.statusCode >= 200 && res.statusCode < 300) return decoded;

    if (res.statusCode == 401 && token != null) onUnauthorized?.call();
    final err =
        (decoded is Map ? decoded['error'] : null) as Map<String, dynamic>?;
    throw ApiException(
      res.statusCode,
      err?['code'] as String? ?? 'unknown',
      _localize(err?['code'] as String?, err?['message'] as String?),
      field: err?['field'] as String?,
    );
  }

  static String _localize(String? code, String? fallback) => switch (code) {
    'invalid_credentials' => 'Неверный логин или пароль',
    'username_taken' => 'Этот логин уже занят',
    'rate_limited' => 'Слишком много попыток, попробуйте через минуту',
    'unauthorized' => 'Сессия истекла, войдите снова',
    'not_found' => 'Не найдено',
    _ => fallback ?? 'Что-то пошло не так',
  };
}
