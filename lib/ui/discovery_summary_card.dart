import 'package:flutter/material.dart';

import 'package:heliolytics/data/models.dart';

class DiscoverySummaryCard extends StatelessWidget {
  final Session session;
  const DiscoverySummaryCard({super.key, required this.session});

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Last session — ${_fmt(session.startedAt)}',
              style: Theme.of(context).textTheme.titleSmall,
            ),
            if (session.deviceMac != null)
              Text(
                'Device: ${session.deviceMac}',
                style: Theme.of(context).textTheme.bodySmall,
              ),
            const SizedBox(height: 12),
            const Text(
              'Chunked:',
              style: TextStyle(fontWeight: FontWeight.w600),
            ),
            ...session.entries.map((e) => _EntryRow(entry: e)),
            const SizedBox(height: 12),
            const Text(
              'Unsolicited / live:',
              style: TextStyle(fontWeight: FontWeight.w600),
            ),
            ...session.unsolicited.map((u) => _UnsolicitedRow(entry: u)),
            if (session.entries.isEmpty && session.unsolicited.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 8),
                child: Text('No entries yet.'),
              ),
          ],
        ),
      ),
    );
  }

  String _fmt(DateTime dt) =>
      '${dt.year}-${dt.month.toString().padLeft(2, '0')}-'
      '${dt.day.toString().padLeft(2, '0')} '
      '${dt.hour.toString().padLeft(2, '0')}:'
      '${dt.minute.toString().padLeft(2, '0')}';
}

class _EntryRow extends StatelessWidget {
  final DumpEntry entry;
  const _EntryRow({required this.entry});

  @override
  Widget build(BuildContext context) {
    final detail = switch (entry.status) {
      DumpStatus.ok => '${entry.samples} samples · ${entry.bytes} bytes',
      DumpStatus.empty => '(empty)',
      DumpStatus.rejected => '(rejected: ${entry.errorByte})',
      DumpStatus.unknown => '(unknown — bytes saved)',
    };
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        children: [
          SizedBox(width: 56, child: Text(entry.code)),
          Expanded(child: Text(detail)),
        ],
      ),
    );
  }
}

class _UnsolicitedRow extends StatelessWidget {
  final UnsolicitedEntry entry;
  const _UnsolicitedRow({required this.entry});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        children: [
          SizedBox(width: 56, child: Text(entry.code)),
          Expanded(child: Text('${entry.count} notifications')),
        ],
      ),
    );
  }
}
