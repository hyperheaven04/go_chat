import 'package:flutter/material.dart';
import 'package:intl/date_symbol_data_local.dart';
import 'package:intl/intl.dart';

import 'state/app_controller.dart';
import 'ui/dialogs_screen.dart';
import 'ui/login_screen.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await initializeDateFormatting('ru');
  Intl.defaultLocale = 'ru';

  final app = AppController();
  runApp(ChatApp(app: app));
  await app.restore();
}

class ChatApp extends StatelessWidget {
  const ChatApp({super.key, required this.app});

  final AppController app;

  ThemeData _theme(Brightness b) => ThemeData(
    colorScheme: ColorScheme.fromSeed(
      seedColor: const Color(0xFF00ADD8),
      brightness: b,
    ),
    useMaterial3: true,
  );

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Go Chat',
      debugShowCheckedModeBanner: false,
      theme: _theme(Brightness.light),
      darkTheme: _theme(Brightness.dark),
      home: ListenableBuilder(
        listenable: app,
        builder: (context, _) {
          if (app.restoring) {
            return const Scaffold(
              body: Center(child: CircularProgressIndicator()),
            );
          }
          final me = app.me;
          return me == null
              ? LoginScreen(app: app)
              : DialogsScreen(key: ValueKey(me.id), app: app);
        },
      ),
    );
  }
}
