import 'package:flutter/foundation.dart';

/// Backend address. Override with `--dart-define=API_URL=http://192.168.1.10:8080`.
class AppConfig {
  static const _override = String.fromEnvironment('API_URL');

  static String get apiUrl {
    if (_override.isNotEmpty) return _override;
    // The Android emulator reaches the host machine via 10.0.2.2.
    if (!kIsWeb && defaultTargetPlatform == TargetPlatform.android) {
      return 'http://10.0.2.2:8080';
    }
    return 'http://localhost:8080';
  }

  static String get wsUrl => apiUrl.replaceFirst(RegExp('^http'), 'ws');
}
