import 'dart:convert';

import '../core_transport/transport_registry.dart';
import '../utils/app_logger.dart';

/// 模型目录拉取结果（来自 Go GetProviderModels）。
class ModelCatalogResult {
  /// 创建成功结果。
  ModelCatalogResult.success({
    required this.models,
    required this.source,
    this.message = '',
  }) : success = true,
       error = '';

  /// 创建失败结果。
  ModelCatalogResult.failure(this.error)
    : success = false,
      models = const [],
      source = '',
      message = '';

  /// 是否成功返回（含官方为空但 HTTP 成功的情况）。
  final bool success;

  /// 错误信息（仅 success == false）。
  final String error;

  /// 模型 id 列表。
  final List<String> models;

  /// 数据来源: official / fallback / static。
  final String source;

  /// 附加说明（如 fallback 原因）。
  final String message;
}

/// 封装 GetProviderModels RPC 与轻量调用日志。
class ModelCatalogService {
  ModelCatalogService._internal();

  static final ModelCatalogService instance = ModelCatalogService._internal();

  /// 异步请求模型列表（桌面走 FFI isolate，Web 走 RPC）。
  Future<ModelCatalogResult> fetchModels({
    required String provider,
    required String baseUrl,
    required String apiKey,
  }) async {
    final transport = TransportRegistry.transport;
    if (!transport.isReady) {
      appLogger.warning('[ModelCatalog] transport not ready');
      return ModelCatalogResult.failure('transport not ready');
    }
    final payload = jsonEncode({
      'provider': provider,
      'base_url': baseUrl,
      'api_key': apiKey,
    });
    try {
      final raw = await transport.callRawOneArgAsync(
        'GetProviderModels',
        payload,
      );
      final decoded = jsonDecode(raw);
      if (decoded is! Map<String, dynamic>) {
        return ModelCatalogResult.failure('invalid response');
      }
      if (decoded['success'] != true) {
        final err = decoded['error']?.toString() ?? 'unknown error';
        return ModelCatalogResult.failure(err);
      }
      final data = decoded['data'];
      if (data is! Map<String, dynamic>) {
        return ModelCatalogResult.success(models: const [], source: 'unknown');
      }
      final rawModels = data['models'];
      final models = <String>[];
      if (rawModels is List) {
        for (final e in rawModels) {
          final s = e?.toString().trim() ?? '';
          if (s.isNotEmpty) {
            models.add(s);
          }
        }
      }
      final source = data['source']?.toString() ?? '';
      final message = data['message']?.toString() ?? '';
      appLogger.info(
        '[ModelCatalog] provider=$provider source=$source count=${models.length}',
      );
      return ModelCatalogResult.success(
        models: models,
        source: source,
        message: message,
      );
    } catch (e, st) {
      appLogger.error('[ModelCatalog] fetch failed', e, st);
      return ModelCatalogResult.failure(e.toString());
    }
  }
}
