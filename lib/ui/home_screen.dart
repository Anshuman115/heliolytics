import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:heliolytics/ble/session_controller.dart';
import 'package:heliolytics/ble/session_state.dart';
import 'package:heliolytics/ui/discovery_summary_card.dart';
import 'package:heliolytics/ui/sessions_screen.dart';

class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final snap = ref.watch(sessionControllerProvider);
    final last = snap.lastSession;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Heliolytics'),
        actions: [
          IconButton(
            icon: const Icon(Icons.folder_open),
            onPressed: () => Navigator.of(context).push(
              MaterialPageRoute<void>(
                builder: (_) => const SessionsScreen(),
              ),
            ),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          _StatePill(state: snap.state),
          const SizedBox(height: 16),
          FilledButton.icon(
            onPressed: snap.state == SessionState.idle
                ? () => ref.read(sessionControllerProvider.notifier).scan()
                : null,
            icon: const Icon(Icons.bluetooth_searching),
            label: const Text('Connect to ring'),
          ),
          const SizedBox(height: 16),
          if (last != null) DiscoverySummaryCard(session: last),
        ],
      ),
    );
  }
}

class _StatePill extends StatelessWidget {
  final SessionState state;
  const _StatePill({required this.state});

  @override
  Widget build(BuildContext context) {
    final color = (state == SessionState.connected ||
            state == SessionState.fetching ||
            state == SessionState.listening)
        ? Colors.green
        : state == SessionState.error
            ? Colors.red
            : Colors.grey;
    return Row(
      children: [
        Container(
          width: 10,
          height: 10,
          decoration: BoxDecoration(color: color, shape: BoxShape.circle),
        ),
        const SizedBox(width: 8),
        Text(_label(state)),
      ],
    );
  }

  String _label(SessionState s) => switch (s) {
        SessionState.noAuthKey => 'No auth key',
        SessionState.idle => 'Idle',
        SessionState.scanning => 'Scanning…',
        SessionState.connecting => 'Connecting…',
        SessionState.authenticating => 'Authenticating…',
        SessionState.connected => 'Connected',
        SessionState.fetching => 'Fetching…',
        SessionState.listening => 'Listening…',
        SessionState.error => 'Error',
      };
}
