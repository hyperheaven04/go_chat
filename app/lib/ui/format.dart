import 'package:intl/intl.dart';

import '../data/models.dart';

bool _sameDay(DateTime a, DateTime b) =>
    a.year == b.year && a.month == b.month && a.day == b.day;

DateTime _yesterday() => DateTime.now().subtract(const Duration(days: 1));

String formatTime(DateTime t) => DateFormat.Hm().format(t);

/// Compact timestamp for the dialog list.
String formatDialogTime(DateTime t) {
  final now = DateTime.now();
  if (_sameDay(t, now)) return formatTime(t);
  if (_sameDay(t, _yesterday())) return 'вчера';
  if (t.year == now.year) return DateFormat('d MMM').format(t);
  return DateFormat('dd.MM.yy').format(t);
}

String formatDay(DateTime t) {
  final now = DateTime.now();
  if (_sameDay(t, now)) return 'Сегодня';
  if (_sameDay(t, _yesterday())) return 'Вчера';
  if (t.year == now.year) return DateFormat('d MMMM').format(t);
  return DateFormat('d MMMM y').format(t);
}

bool isSameDay(DateTime a, DateTime b) => _sameDay(a, b);

String formatPresence(User u) {
  if (u.online) return 'в сети';
  final t = u.lastSeenAt;
  if (t == null) return 'не в сети';
  final now = DateTime.now();
  final diff = now.difference(t);
  if (diff.inMinutes < 1) return 'был(а) только что';
  if (diff.inMinutes < 60) return 'был(а) ${diff.inMinutes} мин. назад';
  if (_sameDay(t, now)) return 'был(а) сегодня в ${formatTime(t)}';
  if (_sameDay(t, _yesterday())) return 'был(а) вчера в ${formatTime(t)}';
  return 'был(а) ${DateFormat('d MMMM').format(t)} в ${formatTime(t)}';
}
