import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:heliolytics/ble/session_controller.dart';
import 'package:heliolytics/ble/session_state.dart';
import 'package:heliolytics/ui/auth_key_screen.dart';
import 'package:heliolytics/ui/home_screen.dart';

class HeliolyticsApp extends ConsumerWidget {
  const HeliolyticsApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp(
      title: 'Heliolytics',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.deepPurple),
        useMaterial3: true,
      ),
      home: const _RootRouter(),
    );
  }
}

class _RootRouter extends ConsumerWidget {
  const _RootRouter();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final snap = ref.watch(sessionControllerProvider);
    if (snap.state == SessionState.noAuthKey) {
      return const AuthKeyScreen();
    }
    return const HomeScreen();
  }
}
