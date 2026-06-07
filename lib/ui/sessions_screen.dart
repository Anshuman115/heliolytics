import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:heliolytics/data/models.dart';
import 'package:heliolytics/data/session_store.dart';
import 'package:heliolytics/ui/discovery_summary_card.dart';

class SessionsScreen extends ConsumerWidget {
  const SessionsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      appBar: AppBar(title: const Text('Sessions')),
      body: FutureBuilder<SessionStore>(
        future: ref.read(sessionStoreProvider.future),
        builder: (context, snap) {
          if (!snap.hasData) {
            return const Center(child: CircularProgressIndicator());
          }
          return FutureBuilder<List<String>>(
            future: snap.data!.listSessions(),
            builder: (context, ids) {
              if (!ids.hasData) {
                return const Center(child: CircularProgressIndicator());
              }
              if (ids.data!.isEmpty) {
                return const Center(child: Text('No sessions yet.'));
              }
              return ListView.builder(
                itemCount: ids.data!.length,
                itemBuilder: (_, i) => _SessionTile(
                  sessionId: ids.data![i],
                  store: snap.data!,
                ),
              );
            },
          );
        },
      ),
    );
  }
}

class _SessionTile extends StatelessWidget {
  final String sessionId;
  final SessionStore store;
  const _SessionTile({required this.sessionId, required this.store});

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<Session>(
      future: store.readSessionJson(sessionId),
      builder: (context, snap) {
        if (!snap.hasData) return const SizedBox.shrink();
        final s = snap.data!;
        return ListTile(
          title: Text(
            '${s.startedAt.year}-${_pad(s.startedAt.month)}-'
            '${_pad(s.startedAt.day)} '
            '${_pad(s.startedAt.hour)}:${_pad(s.startedAt.minute)}',
          ),
          subtitle: Text(
            '${s.deviceMac ?? "?"}  ·  '
            '${s.entries.length} types, ${s.unsolicited.length} unsolicited',
          ),
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (_) => SessionDetailScreen(session: s),
            ),
          ),
        );
      },
    );
  }

  String _pad(int n) => n.toString().padLeft(2, '0');
}

class SessionDetailScreen extends StatelessWidget {
  final Session session;
  const SessionDetailScreen({super.key, required this.session});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('Session ${session.sessionId}')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          DiscoverySummaryCard(session: session),
        ],
      ),
    );
  }
}
