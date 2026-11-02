import 'package:flutter/material.dart';

import '../data/models.dart';
import '../data/realtime.dart';

class UserAvatar extends StatelessWidget {
  const UserAvatar(this.user, {super.key, this.radius = 22});

  final User user;
  final double radius;

  @override
  Widget build(BuildContext context) {
    final hue = (user.username.hashCode % 360).abs().toDouble();
    final bg = HSLColor.fromAHSL(1, hue, 0.45, 0.55).toColor();
    return Stack(
      clipBehavior: Clip.none,
      children: [
        CircleAvatar(
          radius: radius,
          backgroundColor: bg,
          child: Text(
            user.username.characters.first.toUpperCase(),
            style: TextStyle(
              color: Colors.white,
              fontWeight: FontWeight.w600,
              fontSize: radius * 0.8,
            ),
          ),
        ),
        if (user.online)
          Positioned(
            right: 0,
            bottom: 0,
            child: Container(
              width: radius * 0.55,
              height: radius * 0.55,
              decoration: BoxDecoration(
                color: const Color(0xFF34C759),
                shape: BoxShape.circle,
                border: Border.all(
                  color: Theme.of(context).colorScheme.surface,
                  width: 2,
                ),
              ),
            ),
          ),
      ],
    );
  }
}

/// Thin bar shown while the realtime connection is down.
class ConnectionBanner extends StatelessWidget {
  const ConnectionBanner(this.status, {super.key});

  final ValueNotifier<ConnectionStatus> status;

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder(
      valueListenable: status,
      builder: (context, s, _) {
        final scheme = Theme.of(context).colorScheme;
        return AnimatedSize(
          duration: const Duration(milliseconds: 200),
          child: s == ConnectionStatus.connected
              ? const SizedBox(width: double.infinity)
              : Container(
                  width: double.infinity,
                  color: scheme.secondaryContainer,
                  padding: const EdgeInsets.symmetric(vertical: 4),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      SizedBox.square(
                        dimension: 12,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: scheme.onSecondaryContainer,
                        ),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        'Подключение…',
                        style: TextStyle(
                          fontSize: 12,
                          color: scheme.onSecondaryContainer,
                        ),
                      ),
                    ],
                  ),
                ),
        );
      },
    );
  }
}
