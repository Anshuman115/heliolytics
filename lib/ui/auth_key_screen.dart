import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:heliolytics/auth/auth_key_validator.dart';
import 'package:heliolytics/ble/session_controller.dart';
import 'package:heliolytics/ble/session_state.dart';

class AuthKeyScreen extends ConsumerStatefulWidget {
  const AuthKeyScreen({super.key});

  @override
  ConsumerState<AuthKeyScreen> createState() => _AuthKeyScreenState();
}

class _AuthKeyScreenState extends ConsumerState<AuthKeyScreen> {
  final _ctrl = TextEditingController();
  String? _error;
  bool _saving = false;

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final reason = AuthKeyValidator.validate(_ctrl.text);
    if (reason != null) {
      setState(() => _error = reason);
      return;
    }
    setState(() {
      _error = null;
      _saving = true;
    });
    try {
      await ref.read(sessionControllerProvider.notifier).saveAuthKey(_ctrl.text);
    } catch (e) {
      setState(() {
        _error = e.toString();
        _saving = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final snap = ref.watch(sessionControllerProvider);
    if (snap.state != SessionState.noAuthKey) return const SizedBox.shrink();
    return Scaffold(
      appBar: AppBar(title: const Text('Heliolytics')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SizedBox(height: 16),
            const Text(
              'Paste your Helio auth key',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w500),
            ),
            const SizedBox(height: 4),
            const Text('32 hex characters', style: TextStyle(color: Colors.grey)),
            const SizedBox(height: 16),
            TextField(
              controller: _ctrl,
              maxLength: 32,
              autocorrect: false,
              enableSuggestions: false,
              decoration: InputDecoration(
                border: const OutlineInputBorder(),
                hintText: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6',
                errorText: _error,
              ),
            ),
            const SizedBox(height: 16),
            FilledButton(
              onPressed: _saving ? null : _save,
              child: _saving
                  ? const SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('Save'),
            ),
            const SizedBox(height: 24),
            TextButton(
              onPressed: () => showDialog<void>(
                context: context,
                builder: (_) => const AlertDialog(
                  title: Text('Where do I find this?'),
                  content: Text(
                    'The auth key is a 32-char hex string associated with '
                    'your ring. It can be retrieved from Zepp, from your '
                    "Zepp Health cloud, or from the Zepp mobile app "
                    'API. Once you have it, paste it here and it will be '
                    'stored in the Android Keystore.',
                  ),
                ),
              ),
              child: const Text('Where do I find this?'),
            ),
          ],
        ),
      ),
    );
  }
}
