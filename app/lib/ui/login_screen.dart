import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../state/app_controller.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key, required this.app});

  final AppController app;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _form = GlobalKey<FormState>();
  final _username = TextEditingController();
  final _password = TextEditingController();
  bool _register = false;
  bool _busy = false;
  bool _obscure = true;
  String? _error;
  String? _usernameError;
  String? _passwordError;

  static final _usernameRe = RegExp(r'^[a-zA-Z0-9_]{3,32}$');

  Future<void> _submit() async {
    setState(() => _error = _usernameError = _passwordError = null);
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    try {
      final u = _username.text.trim(), p = _password.text;
      _register
          ? await widget.app.register(u, p)
          : await widget.app.login(u, p);
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(
        () => switch (e.field) {
          'username' => _usernameError = e.message,
          'password' => _passwordError = e.message,
          _ => _error = e.message,
        },
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 400),
              child: Form(
                key: _form,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Icon(
                      Icons.forum_rounded,
                      size: 64,
                      color: theme.colorScheme.primary,
                    ),
                    const SizedBox(height: 12),
                    Text(
                      'Go Chat',
                      textAlign: TextAlign.center,
                      style: theme.textTheme.headlineMedium?.copyWith(
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 32),
                    SegmentedButton<bool>(
                      segments: const [
                        ButtonSegment(value: false, label: Text('Вход')),
                        ButtonSegment(value: true, label: Text('Регистрация')),
                      ],
                      selected: {_register},
                      onSelectionChanged: (s) => setState(() {
                        _register = s.first;
                        _error = _usernameError = _passwordError = null;
                      }),
                    ),
                    const SizedBox(height: 24),
                    TextFormField(
                      controller: _username,
                      autofillHints: const [AutofillHints.username],
                      textInputAction: TextInputAction.next,
                      autocorrect: false,
                      decoration: InputDecoration(
                        labelText: 'Логин',
                        prefixIcon: const Icon(Icons.person_outline),
                        border: const OutlineInputBorder(),
                        helperText: _register
                            ? 'Латиница, цифры и _, от 3 до 32 символов'
                            : null,
                        errorText: _usernameError,
                      ),
                      validator: (v) =>
                          _register && !_usernameRe.hasMatch(v!.trim())
                          ? 'Латиница, цифры и _, от 3 до 32 символов'
                          : (v!.trim().isEmpty ? 'Введите логин' : null),
                    ),
                    const SizedBox(height: 16),
                    TextFormField(
                      controller: _password,
                      obscureText: _obscure,
                      autofillHints: [
                        _register
                            ? AutofillHints.newPassword
                            : AutofillHints.password,
                      ],
                      onFieldSubmitted: (_) => _submit(),
                      decoration: InputDecoration(
                        labelText: 'Пароль',
                        prefixIcon: const Icon(Icons.lock_outline),
                        border: const OutlineInputBorder(),
                        helperText: _register ? 'Минимум 8 символов' : null,
                        errorText: _passwordError,
                        suffixIcon: IconButton(
                          icon: Icon(
                            _obscure ? Icons.visibility : Icons.visibility_off,
                          ),
                          onPressed: () => setState(() => _obscure = !_obscure),
                        ),
                      ),
                      validator: (v) => _register && v!.length < 8
                          ? 'Минимум 8 символов'
                          : (v!.isEmpty ? 'Введите пароль' : null),
                    ),
                    if (_error != null) ...[
                      const SizedBox(height: 16),
                      Text(
                        _error!,
                        textAlign: TextAlign.center,
                        style: TextStyle(color: theme.colorScheme.error),
                      ),
                    ],
                    const SizedBox(height: 24),
                    FilledButton(
                      onPressed: _busy ? null : _submit,
                      style: FilledButton.styleFrom(
                        minimumSize: const Size.fromHeight(52),
                      ),
                      child: _busy
                          ? const SizedBox.square(
                              dimension: 22,
                              child: CircularProgressIndicator(
                                strokeWidth: 2.5,
                              ),
                            )
                          : Text(_register ? 'Создать аккаунт' : 'Войти'),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  @override
  void dispose() {
    _username.dispose();
    _password.dispose();
    super.dispose();
  }
}
