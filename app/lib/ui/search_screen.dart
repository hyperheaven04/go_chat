import 'dart:async';

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import '../state/app_controller.dart';
import 'format.dart';
import 'widgets.dart';

/// Finds a user and opens (or creates) the dialog with them.
/// Pops with the resulting [Conversation].
class SearchScreen extends StatefulWidget {
  const SearchScreen({super.key, required this.app});

  final AppController app;

  @override
  State<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends State<SearchScreen> {
  Timer? _debounce;
  List<User> _results = [];
  bool _loading = false;
  String _query = '';

  void _onChanged(String q) {
    _debounce?.cancel();
    _debounce = Timer(
      const Duration(milliseconds: 300),
      () => _search(q.trim()),
    );
  }

  Future<void> _search(String q) async {
    setState(() {
      _query = q;
      _loading = q.isNotEmpty;
    });
    if (q.isEmpty) {
      setState(() => _results = []);
      return;
    }
    try {
      final r = await widget.app.api.searchUsers(q);
      if (mounted && q == _query) setState(() => _results = r);
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(e.message)));
      }
    } finally {
      if (mounted && q == _query) setState(() => _loading = false);
    }
  }

  Future<void> _open(User u) async {
    try {
      final d = await widget.app.api.openDialog(u.id);
      if (mounted) Navigator.pop(context, d);
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(e.message)));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: TextField(
          autofocus: true,
          autocorrect: false,
          onChanged: _onChanged,
          decoration: const InputDecoration(
            hintText: 'Поиск по логину',
            border: InputBorder.none,
          ),
        ),
        bottom: _loading
            ? const PreferredSize(
                preferredSize: Size.fromHeight(2),
                child: LinearProgressIndicator(minHeight: 2),
              )
            : null,
      ),
      body: _query.isNotEmpty && _results.isEmpty && !_loading
          ? const Center(child: Text('Никого не нашли'))
          : ListView.builder(
              itemCount: _results.length,
              itemBuilder: (context, i) {
                final u = _results[i];
                return ListTile(
                  leading: UserAvatar(u),
                  title: Text(u.username),
                  subtitle: Text(formatPresence(u)),
                  onTap: () => _open(u),
                );
              },
            ),
    );
  }

  @override
  void dispose() {
    _debounce?.cancel();
    super.dispose();
  }
}
