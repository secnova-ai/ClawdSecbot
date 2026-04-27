import 'package:flutter/material.dart';
import 'package:fl_chart/fl_chart.dart';
import 'package:lucide_icons/lucide_icons.dart';
import '../l10n/app_localizations.dart';
import '../models/protection_analysis_model.dart';
import '../utils/app_fonts.dart';

/// 防护监控统计和图表辅助类
/// 包含统计卡片、API 指标、Token 趋势图、Tool Call 趋势图
/// 注意：这不是一个直接使用的 Widget，而是通过各 build 方法由父组件调用
class ProtectionMonitorCharts {
  final int analysisCount;
  final int messageCount;
  final int warningCount;
  final int blockedCount;
  final int totalPromptTokens;
  final int totalCompletionTokens;
  final int totalToolCalls;
  final int auditPromptTokens;
  final int auditCompletionTokens;
  final ApiStatistics? statistics;

  ProtectionMonitorCharts({
    required this.analysisCount,
    required this.messageCount,
    required this.warningCount,
    required this.blockedCount,
    required this.totalPromptTokens,
    required this.totalCompletionTokens,
    required this.totalToolCalls,
    required this.auditPromptTokens,
    required this.auditCompletionTokens,
    this.statistics,
  });

  // ============ 统计卡片行 ============

  Widget buildStatisticsRow(AppLocalizations l10n) {
    return Row(
      children: [
        Expanded(
          child: _buildStatCard(
            l10n.analysisCount,
            analysisCount.toString(),
            LucideIcons.scan,
            const Color(0xFF6366F1),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: _buildStatCard(
            l10n.messageCountLabel,
            messageCount.toString(),
            LucideIcons.messageSquare,
            const Color(0xFF8B5CF6),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: _buildStatCard(
            l10n.warningCountLabel,
            warningCount.toString(),
            LucideIcons.alertTriangle,
            const Color(0xFFF59E0B),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: _buildStatCard(
            l10n.blockedCount,
            blockedCount.toString(),
            LucideIcons.shieldOff,
            blockedCount > 0
                ? const Color(0xFFEF4444)
                : const Color(0xFF22C55E),
          ),
        ),
      ],
    );
  }

  // ============ API 指标行 ============

  Widget buildApiMetricsRow(AppLocalizations l10n) {
    final calculatedTotalTokens = totalPromptTokens + totalCompletionTokens;

    return Row(
      children: [
        Expanded(
          child: _buildStatCard(
            l10n.totalTokens,
            _formatNumber(calculatedTotalTokens),
            LucideIcons.coins,
            const Color(0xFF10B981),
            tooltip: l10n.totalTokenTooltip,
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: _buildStatCard(
            l10n.promptTokens,
            _formatNumber(totalPromptTokens),
            LucideIcons.arrowUpCircle,
            const Color(0xFF3B82F6),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: _buildStatCard(
            l10n.completionTokens,
            _formatNumber(totalCompletionTokens),
            LucideIcons.arrowDownCircle,
            const Color(0xFF8B5CF6),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: _buildStatCard(
            l10n.toolCallCount,
            totalToolCalls.toString(),
            LucideIcons.wrench,
            const Color(0xFFEC4899),
          ),
        ),
      ],
    );
  }

  // ============ 分析指标行 ============

  Widget buildAnalysisMetricsRow(AppLocalizations l10n) {
    final calculatedAuditTokens = auditPromptTokens + auditCompletionTokens;

    return Container(
      margin: const EdgeInsets.only(top: 12),
      child: Row(
        children: [
          Expanded(
            child: _buildStatCard(
              l10n.analysisTokens,
              _formatNumber(calculatedAuditTokens),
              LucideIcons.shieldAlert,
              const Color(0xFF6366F1),
              tooltip: l10n.analysisTokenTooltip,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: _buildStatCard(
              l10n.analysisPromptTokens,
              _formatNumber(auditPromptTokens),
              LucideIcons.arrowUpCircle,
              const Color(0xFF8B5CF6),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: _buildStatCard(
              l10n.analysisCompletionTokens,
              _formatNumber(auditCompletionTokens),
              LucideIcons.arrowDownCircle,
              const Color(0xFFEC4899),
            ),
          ),
          const SizedBox(width: 12),
          const Expanded(child: SizedBox()),
        ],
      ),
    );
  }

  // ============ Token 趋势图 ============

  Widget buildTokenTrendChart(AppLocalizations l10n) {
    final tokenTrend = statistics?.tokenTrend ?? [];
    final tokenInterval = _calculateInterval(tokenTrend);

    return RepaintBoundary(
      child: Container(
        decoration: BoxDecoration(
          color: Colors.white.withValues(alpha: 0.05),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.all(12),
              child: Row(
                children: [
                  const Icon(
                    LucideIcons.trendingUp,
                    color: Color(0xFF10B981),
                    size: 16,
                  ),
                  const SizedBox(width: 8),
                  Text(
                    l10n.tokenTrend,
                    style: AppFonts.inter(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                      color: Colors.white,
                    ),
                  ),
                ],
              ),
            ),
            Divider(height: 1, color: Colors.white.withValues(alpha: 0.1)),
            Expanded(
              child: tokenTrend.isEmpty
                  ? Center(
                      child: Text(
                        l10n.noDataYet,
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: Colors.white38,
                        ),
                      ),
                    )
                  : Padding(
                      padding: const EdgeInsets.all(12),
                      child: LineChart(
                        LineChartData(
                          gridData: FlGridData(
                            show: true,
                            drawVerticalLine: false,
                            horizontalInterval: tokenInterval,
                            getDrawingHorizontalLine: (value) {
                              return FlLine(
                                color: Colors.white.withValues(alpha: 0.1),
                                strokeWidth: 1,
                              );
                            },
                          ),
                          titlesData: FlTitlesData(
                            show: true,
                            rightTitles: const AxisTitles(
                              sideTitles: SideTitles(showTitles: false),
                            ),
                            topTitles: const AxisTitles(
                              sideTitles: SideTitles(showTitles: false),
                            ),
                            bottomTitles: AxisTitles(
                              sideTitles: SideTitles(
                                showTitles: true,
                                reservedSize: 22,
                                interval: _calculateTimeInterval(
                                  tokenTrend.length,
                                ),
                                getTitlesWidget: (value, meta) {
                                  final index = value.toInt();
                                  if (index < 0 || index >= tokenTrend.length) {
                                    return const SizedBox.shrink();
                                  }
                                  final time = tokenTrend[index].timestamp;
                                  return Text(
                                    '${time.hour}:${time.minute.toString().padLeft(2, '0')}',
                                    style: AppFonts.firaCode(
                                      fontSize: 9,
                                      color: Colors.white38,
                                    ),
                                  );
                                },
                              ),
                            ),
                            leftTitles: AxisTitles(
                              sideTitles: SideTitles(
                                showTitles: true,
                                reservedSize: 40,
                                interval: tokenInterval,
                                getTitlesWidget: (value, meta) {
                                  return Text(
                                    _formatNumber(value.toInt()),
                                    style: AppFonts.firaCode(
                                      fontSize: 9,
                                      color: Colors.white38,
                                    ),
                                  );
                                },
                              ),
                            ),
                          ),
                          borderData: FlBorderData(show: false),
                          minX: 0,
                          maxX: (tokenTrend.length - 1).toDouble(),
                          minY: 0,
                          maxY: _calculateTokenMaxY(tokenTrend),
                          lineTouchData: LineTouchData(
                            handleBuiltInTouches: true,
                            touchTooltipData: LineTouchTooltipData(
                              getTooltipColor: (_) =>
                                  const Color(0xFF1A1A2E).withValues(alpha: 0.95),
                              tooltipRoundedRadius: 6,
                              getTooltipItems: (spots) => spots.map((spot) {
                                return LineTooltipItem(
                                  _formatNumber(spot.y.toInt()),
                                  AppFonts.firaCode(
                                    fontSize: 10,
                                    color: const Color(0xFF10B981),
                                  ),
                                );
                              }).toList(),
                            ),
                          ),
                          lineBarsData: [
                            LineChartBarData(
                              spots: tokenTrend.asMap().entries.map((e) {
                                // 当值为 0 时使用极小正值，确保曲线不会下弯
                                final value = e.value.tokens;
                                return FlSpot(
                                  e.key.toDouble(),
                                  value == 0 ? 0.5 : value.toDouble(),
                                );
                              }).toList(),
                              isCurved: true,
                              curveSmoothness: 0.3,
                              color: const Color(0xFF10B981),
                              barWidth: 2,
                              isStrokeCapRound: true,
                              dotData: const FlDotData(show: false),
                              belowBarData: BarAreaData(
                                show: true,
                                color: const Color(0xFF10B981)
                                    .withValues(alpha: 0.1),
                                cutOffY: 0,
                                applyCutOffY: true,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  // ============ Tool Call 趋势图 ============

  Widget buildToolCallChart(AppLocalizations l10n) {
    final toolCallTrend = statistics?.toolCallTrend ?? [];

    return RepaintBoundary(
      child: Container(
        decoration: BoxDecoration(
          color: Colors.white.withValues(alpha: 0.05),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: Colors.white.withValues(alpha: 0.1)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.all(12),
              child: Row(
                children: [
                  const Icon(
                    LucideIcons.wrench,
                    color: Color(0xFFEC4899),
                    size: 16,
                  ),
                  const SizedBox(width: 8),
                  Text(
                    l10n.toolCallTrend,
                    style: AppFonts.inter(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                      color: Colors.white,
                    ),
                  ),
                ],
              ),
            ),
            Divider(height: 1, color: Colors.white.withValues(alpha: 0.1)),
            Expanded(
              child: toolCallTrend.isEmpty
                  ? Center(
                      child: Text(
                        l10n.noDataYet,
                        style: AppFonts.inter(
                          fontSize: 12,
                          color: Colors.white38,
                        ),
                      ),
                    )
                  : Padding(
                      padding: const EdgeInsets.all(12),
                      child: BarChart(
                        BarChartData(
                          minY: 0,
                          maxY: _calculateToolCallMaxY(toolCallTrend),
                          gridData: FlGridData(
                            show: true,
                            drawVerticalLine: false,
                            horizontalInterval: _calculateToolCallInterval(
                              toolCallTrend,
                            ),
                            getDrawingHorizontalLine: (value) {
                              return FlLine(
                                color: Colors.white.withValues(alpha: 0.1),
                                strokeWidth: 1,
                              );
                            },
                          ),
                          titlesData: FlTitlesData(
                            show: true,
                            rightTitles: const AxisTitles(
                              sideTitles: SideTitles(showTitles: false),
                            ),
                            topTitles: const AxisTitles(
                              sideTitles: SideTitles(showTitles: false),
                            ),
                            bottomTitles: AxisTitles(
                              sideTitles: SideTitles(
                                showTitles: true,
                                reservedSize: 22,
                                getTitlesWidget: (value, meta) {
                                  final index = value.toInt();
                                  if (index < 0 ||
                                      index >= toolCallTrend.length) {
                                    return const SizedBox.shrink();
                                  }
                                  if (index %
                                          _calculateTimeInterval(
                                            toolCallTrend.length,
                                          ).toInt() !=
                                      0) {
                                    return const SizedBox.shrink();
                                  }
                                  final time = toolCallTrend[index].timestamp;
                                  return Text(
                                    '${time.hour}:${time.minute.toString().padLeft(2, '0')}',
                                    style: AppFonts.firaCode(
                                      fontSize: 9,
                                      color: Colors.white38,
                                    ),
                                  );
                                },
                              ),
                            ),
                            leftTitles: AxisTitles(
                              sideTitles: SideTitles(
                                showTitles: true,
                                reservedSize: 30,
                                interval: _calculateToolCallInterval(
                                  toolCallTrend,
                                ),
                                getTitlesWidget: (value, meta) {
                                  return Text(
                                    value.toInt().toString(),
                                    style: AppFonts.firaCode(
                                      fontSize: 9,
                                      color: Colors.white38,
                                    ),
                                  );
                                },
                              ),
                            ),
                          ),
                          borderData: FlBorderData(show: false),
                          barTouchData: BarTouchData(
                            touchTooltipData: BarTouchTooltipData(
                              getTooltipColor: (_) =>
                                  const Color(0xFF1A1A2E).withValues(alpha: 0.95),
                              tooltipRoundedRadius: 6,
                              getTooltipItem: (group, groupIndex, rod, rodIndex) {
                                return BarTooltipItem(
                                  rod.toY.toInt().toString(),
                                  AppFonts.firaCode(
                                    fontSize: 10,
                                    color: const Color(0xFFEC4899),
                                  ),
                                );
                              },
                            ),
                          ),
                          barGroups: toolCallTrend.asMap().entries.map((e) {
                            return BarChartGroupData(
                              x: e.key,
                              barRods: [
                                BarChartRodData(
                                  toY: e.value.count.toDouble(),
                                  color: const Color(0xFFEC4899),
                                  width: toolCallTrend.length > 20 ? 4 : 8,
                                  borderRadius: const BorderRadius.only(
                                    topLeft: Radius.circular(2),
                                    topRight: Radius.circular(2),
                                  ),
                                ),
                              ],
                            );
                          }).toList(),
                        ),
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  // ============ 辅助方法 ============

  Widget _buildStatCard(
    String label,
    String value,
    IconData icon,
    Color color, {
    String? tooltip,
  }) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: color.withValues(alpha: 0.2)),
      ),
      child: Row(
        children: [
          Icon(icon, color: color, size: 16),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  crossAxisAlignment: CrossAxisAlignment.baseline,
                  textBaseline: TextBaseline.alphabetic,
                  children: [
                    Text(
                      value,
                      style: AppFonts.firaCode(
                        fontSize: 18,
                        fontWeight: FontWeight.bold,
                        color: Colors.white,
                      ),
                    ),
                  ],
                ),
                Row(
                  children: [
                    Flexible(
                      child: Text(
                        label,
                        style: AppFonts.inter(
                          fontSize: 10,
                          color: Colors.white54,
                        ),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    if (tooltip != null) ...[
                      const SizedBox(width: 4),
                      Tooltip(
                        message: tooltip,
                        child: const Icon(
                          LucideIcons.info,
                          size: 10,
                          color: Colors.white38,
                        ),
                      ),
                    ],
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  String _formatNumber(int number) {
    if (number >= 1000000) {
      return '${(number / 1000000).toStringAsFixed(1)}M';
    } else if (number >= 1000) {
      return '${(number / 1000).toStringAsFixed(1)}K';
    }
    return number.toString();
  }

  double _calculateInterval(List<TokenTrendPoint> data) {
    if (data.isEmpty) return 100;
    final maxValue = data.map((e) => e.tokens).reduce((a, b) => a > b ? a : b);
    if (maxValue <= 0) return 100;
    return (maxValue / 4).ceilToDouble();
  }

  double _calculateTimeInterval(int dataLength) {
    if (dataLength <= 5) return 1;
    if (dataLength <= 10) return 2;
    if (dataLength <= 20) return 4;
    return (dataLength / 5).ceilToDouble();
  }

  double _calculateToolCallInterval(List<ToolCallTrendPoint> data) {
    if (data.isEmpty) return 1;
    final maxValue = data.map((e) => e.count).reduce((a, b) => a > b ? a : b);
    if (maxValue <= 0) return 1;
    if (maxValue <= 4) return 1;
    return (maxValue / 4).ceilToDouble();
  }

  /// 计算 Token 趋势图纵轴上限，避免自动刻度导致标签重复重影
  double _calculateTokenMaxY(List<TokenTrendPoint> data) {
    if (data.isEmpty) return 100;
    final maxValue = data.map((e) => e.tokens).reduce((a, b) => a > b ? a : b);
    if (maxValue <= 0) return 100;
    final interval = _calculateInterval(data);
    final maxY = interval * 4;
    return maxY >= maxValue ? maxY : (maxValue / interval).ceil() * interval;
  }

  /// 计算工具调用趋势图纵轴上限，使刻度与网格间隔对齐，比例正确
  double _calculateToolCallMaxY(List<ToolCallTrendPoint> data) {
    if (data.isEmpty) return 4;
    final maxValue = data.map((e) => e.count).reduce((a, b) => a > b ? a : b);
    if (maxValue <= 0) return 4;
    final interval = _calculateToolCallInterval(data);
    final maxY = interval * 4;
    return maxY >= maxValue ? maxY : (maxValue / interval).ceil() * interval;
  }

}
