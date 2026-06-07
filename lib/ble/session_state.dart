import 'package:heliolytics/data/models.dart';

enum SessionState {
  noAuthKey,
  idle,
  scanning,
  connecting,
  authenticating,
  connected,
  fetching,
  listening,
  error,
}

enum SessionError {
  none,
  scanTimeout,
  scanFailed,
  gattFailed,
  authRejected,
  authTimeout,
  chunkTimeout,
  bleDisconnected,
  unknown,
}

class SessionSnapshot {
  final SessionState state;
  final SessionError error;
  final String? currentTypeCode;
  final Session? lastSession;
  final String? lastErrorMessage;

  const SessionSnapshot({
    required this.state,
    required this.error,
    this.currentTypeCode,
    this.lastSession,
    this.lastErrorMessage,
  });

  static const initial = SessionSnapshot(
    state: SessionState.noAuthKey,
    error: SessionError.none,
  );

  SessionSnapshot copyWith({
    SessionState? state,
    SessionError? error,
    String? currentTypeCode,
    Session? lastSession,
    String? lastErrorMessage,
  }) =>
      SessionSnapshot(
        state: state ?? this.state,
        error: error ?? this.error,
        currentTypeCode: currentTypeCode ?? this.currentTypeCode,
        lastSession: lastSession ?? this.lastSession,
        lastErrorMessage: lastErrorMessage ?? this.lastErrorMessage,
      );
}
