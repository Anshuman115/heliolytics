enum DumpStatus { ok, empty, rejected, unknown }

extension DumpStatusX on DumpStatus {
  String get label => switch (this) {
        DumpStatus.ok => 'ok',
        DumpStatus.empty => 'empty',
        DumpStatus.rejected => 'rejected',
        DumpStatus.unknown => 'unknown',
      };
  static DumpStatus parse(String s) => switch (s) {
        'ok' => DumpStatus.ok,
        'empty' => DumpStatus.empty,
        'rejected' => DumpStatus.rejected,
        'unknown' => DumpStatus.unknown,
        _ => throw FormatException('Unknown DumpStatus: $s'),
      };
}

enum SessionMode { fetchAndListen, listenOnly }

extension SessionModeX on SessionMode {
  String get label => switch (this) {
        SessionMode.fetchAndListen => 'fetch+listen',
        SessionMode.listenOnly => 'listen-only',
      };
  static SessionMode parse(String s) => switch (s) {
        'fetch+listen' => SessionMode.fetchAndListen,
        'listen-only' => SessionMode.listenOnly,
        _ => throw FormatException('Unknown SessionMode: $s'),
      };
}

class DumpEntry {
  final String code;
  final DumpStatus status;
  final int samples, bytes;
  final String? file;
  final String? errorByte;

  const DumpEntry({
    required this.code,
    required this.status,
    required this.samples,
    required this.bytes,
    this.file,
    this.errorByte,
  });

  Map<String, dynamic> toJson() => {
        'schemaVersion': 1,
        'code': code,
        'status': status.label,
        'samples': samples,
        'bytes': bytes,
        if (file != null) 'file': file,
        if (errorByte != null) 'errorByte': errorByte,
      };

  factory DumpEntry.fromJson(Map<String, dynamic> j) => DumpEntry(
        code: j['code'] as String,
        status: DumpStatusX.parse(j['status'] as String),
        samples: (j['samples'] as num).toInt(),
        bytes: (j['bytes'] as num).toInt(),
        file: j['file'] as String?,
        errorByte: j['errorByte'] as String?,
      );
}

class UnsolicitedEntry {
  final String code, kind, file;
  final int count;
  final String? firstBytesHex;

  const UnsolicitedEntry({
    required this.code,
    required this.kind,
    required this.count,
    required this.file,
    this.firstBytesHex,
  });

  Map<String, dynamic> toJson() => {
        'code': code,
        'kind': kind,
        'count': count,
        'file': file,
        if (firstBytesHex != null) 'firstBytesHex': firstBytesHex,
      };

  factory UnsolicitedEntry.fromJson(Map<String, dynamic> j) => UnsolicitedEntry(
        code: j['code'] as String,
        kind: j['kind'] as String,
        count: (j['count'] as num).toInt(),
        file: j['file'] as String,
        firstBytesHex: j['firstBytesHex'] as String?,
      );
}

class Session {
  final String sessionId;
  final DateTime startedAt;
  final DateTime? endedAt;
  final String? deviceMac;
  final int fetchWindowHours, listenDurationSec;
  final SessionMode mode;
  final List<DumpEntry> entries;
  final List<UnsolicitedEntry> unsolicited;

  const Session({
    required this.sessionId,
    required this.startedAt,
    this.endedAt,
    this.deviceMac,
    required this.fetchWindowHours,
    required this.listenDurationSec,
    required this.mode,
    required this.entries,
    required this.unsolicited,
  });

  Map<String, dynamic> toJson() => {
        'schemaVersion': 1,
        'sessionId': sessionId,
        'startedAt': startedAt.toIso8601String(),
        if (endedAt != null) 'endedAt': endedAt!.toIso8601String(),
        if (deviceMac != null) 'deviceMac': deviceMac,
        'fetchWindowHours': fetchWindowHours,
        'listenDurationSec': listenDurationSec,
        'mode': mode.label,
      };

  factory Session.fromJson(Map<String, dynamic> j) => Session(
        sessionId: j['sessionId'] as String,
        startedAt: DateTime.parse(j['startedAt'] as String),
        endedAt:
            j['endedAt'] != null ? DateTime.parse(j['endedAt'] as String) : null,
        deviceMac: j['deviceMac'] as String?,
        fetchWindowHours: (j['fetchWindowHours'] as num).toInt(),
        listenDurationSec: (j['listenDurationSec'] as num).toInt(),
        mode: SessionModeX.parse(j['mode'] as String),
        entries: (j['entries'] as List<dynamic>? ?? [])
            .map((e) => DumpEntry.fromJson(e as Map<String, dynamic>))
            .toList(),
        unsolicited: (j['unsolicited'] as List<dynamic>? ?? [])
            .map((e) => UnsolicitedEntry.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class SessionCatalog {
  static const int schemaVersion = 1;
  final String sessionId;
  final List<DumpEntry> chunked;
  final List<UnsolicitedEntry> unsolicited;

  const SessionCatalog({
    required this.sessionId,
    required this.chunked,
    required this.unsolicited,
  });

  Map<String, dynamic> toJson() => {
        'schemaVersion': schemaVersion,
        'sessionId': sessionId,
        'chunked': chunked.map((e) => e.toJson()).toList(),
        'unsolicited': unsolicited.map((e) => e.toJson()).toList(),
      };

  factory SessionCatalog.fromJson(Map<String, dynamic> j) => SessionCatalog(
        sessionId: j['sessionId'] as String,
        chunked: (j['chunked'] as List<dynamic>)
            .map((e) => DumpEntry.fromJson(e as Map<String, dynamic>))
            .toList(),
        unsolicited: (j['unsolicited'] as List<dynamic>)
            .map((e) => UnsolicitedEntry.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}
