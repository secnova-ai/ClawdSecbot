import 'dart:io';

bool get isRuntimeLinux => Platform.isLinux;
bool get isRuntimeMacOS => Platform.isMacOS;
bool get isRuntimeWindows => Platform.isWindows;
bool get isRuntimeWeb => false;
Map<String, String> get runtimeEnvironment => Platform.environment;
