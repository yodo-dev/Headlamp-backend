import 'package:flutter/material.dart';
import 'package:onesignal_flutter/onesignal_flutter.dart';
import 'package:http/http.dart' as http;
import 'dart:convert';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatefulWidget {
  const MyApp({super.key});

  @override
  State<MyApp> createState() => _MyAppState();
}

class _MyAppState extends State<MyApp> {
  String _playerId = 'Not available';
  String _receivedNotification = 'No notification received yet.';
  final TextEditingController _tokenController = TextEditingController();

  // TODO: Replace with your OneSignal App ID
  static const String oneSignalAppId = '';

  @override
  void initState() {
    super.initState();
    initOneSignal();
  }

  Future<void> initOneSignal() async {
    OneSignal.Debug.setLogLevel(OSLogLevel.verbose);
    OneSignal.initialize(oneSignalAppId);
    OneSignal.Notifications.requestPermission(true);

    OneSignal.User.pushSubscription.addObserver((state) {
      setState(() {
        _playerId = state.current.id ?? 'Not available';
      });
    });

    OneSignal.Notifications.addClickListener((event) {
      setState(() {
        _receivedNotification =
            'Notification clicked: ${event.notification.title}';
      });
    });
  }

  Future<void> registerDevice() async {
    final token = _tokenController.text;
    if (token.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Please enter an auth token.')),
      );
      return;
    }

    // TODO: Replace with your backend URL
    final url =
        Uri.parse('http://localhost:8080/v1/notifications/device/register');
    try {
      final response = await http.post(
        url,
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer $token',
        },
        body: json.encode({'player_id': _playerId}),
      );

      if (response.statusCode == 200) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Device registered successfully!')),
        );
      } else {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
              content: Text('Failed to register device: ${response.body}')),
        );
      }
    } catch (e) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Error: $e')),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      home: Scaffold(
        appBar: AppBar(
          title: const Text('OneSignal Test App'),
        ),
        body: Padding(
          padding: EdgeInsets.all(16.0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Player ID: $_playerId'),
              const SizedBox(height: 20),
              TextField(
                controller: _tokenController,
                decoration: const InputDecoration(
                  labelText: 'Enter Auth Token',
                ),
              ),
              const SizedBox(height: 20),
              ElevatedButton(
                onPressed: registerDevice,
                child: const Text('Register Device'),
              ),
              const SizedBox(height: 40),
              Text('Last Notification: $_receivedNotification'),
            ],
          ),
        ),
      ),
    );
  }
}
