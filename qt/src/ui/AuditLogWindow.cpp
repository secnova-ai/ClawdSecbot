#include "ui/AuditLogWindow.h"

#include "bridge/GoBridge.h"
#include "service/AuditService.h"
#include "ui/widgets/AuditTimelineWidget.h"
#include "ui/widgets/GradientWidget.h"

#include <QCheckBox>
#include <QButtonGroup>
#include <QApplication>
#include <QClipboard>
#include <QDialog>
#include <QDateTime>
#include <QFileDialog>
#include <QFutureWatcher>
#include <QFrame>
#include <QHBoxLayout>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QLineEdit>
#include <QListWidget>
#include <QMessageBox>
#include <QPushButton>
#include <QSaveFile>
#include <QScrollArea>
#include <QSplitter>
#include <QVBoxLayout>
#include <QtConcurrent>

#include <algorithm>

namespace {
QByteArray exportMarkdown(const QList<AuditLogModel>& selectedLogs) {
    QByteArray markdown("# 审计日志批量导出\n\n");
    for (const AuditLogModel& log : selectedLogs) {
        markdown += QStringLiteral("## %1 · %2\n\n").arg(log.timestamp, log.id).toUtf8();
        markdown += QStringLiteral("- Bot: %1\n- 资产 ID: %2\n- 请求 ID: %3\n- 模型: %4\n- 动作: %5\n- 风险: %6\n- 耗时: %7 ms\n- Token: %8\n\n")
                        .arg(log.assetName, log.assetId, log.requestId, log.model, log.action,
                             log.riskLevel.isEmpty() ? QStringLiteral("无") : log.riskLevel,
                             QString::number(log.durationMs), QString::number(log.totalTokens))
                        .toUtf8();
        markdown += QStringLiteral("### 请求\n\n%1\n\n### 输出\n\n%2\n\n")
                        .arg(log.requestContent, log.outputContent)
                        .toUtf8();
        if (!log.messages.isEmpty()) {
            markdown += "### 消息链\n\n";
            for (const AuditMessageModel& message : log.messages) {
                markdown += QStringLiteral("- `%1` %2\n").arg(message.role, message.content).toUtf8();
            }
            markdown += '\n';
        }
        if (!log.toolCalls.isEmpty()) {
            markdown += "### 工具调用\n\n";
            for (const AuditToolCallModel& call : log.toolCalls) {
                markdown += QStringLiteral("- `%1`\n\n  参数: %2\n\n  结果: %3\n")
                                .arg(call.name, call.arguments, call.result)
                                .toUtf8();
            }
            markdown += '\n';
        }
    }
    return markdown;
}
}

AuditLogWindow::AuditLogWindow(GoBridge* bridge, QWidget* parent)
    : QMainWindow(parent), bridge_(bridge) {
    setAttribute(Qt::WA_DeleteOnClose);
    setWindowTitle(QStringLiteral("审计日志"));
    setMinimumSize(1024, 600);
    resize(1280, 800);
    buildUi();
    loadAssetTabs();
    refresh();
}

void AuditLogWindow::buildUi() {
    auto* shell = new GradientWidget(this);
    auto* root = new QVBoxLayout(shell);
    root->setContentsMargins(0, 0, 0, 0);
    root->setSpacing(0);

    auto* titleBar = new QWidget(shell);
    titleBar->setObjectName(QStringLiteral("titleBar"));
    titleBar->setFixedHeight(48);
    auto* titleLayout = new QHBoxLayout(titleBar);
    titleLayout->setContentsMargins(16, 0, 16, 0);
    auto* icon = new QLabel(QStringLiteral("▤"), titleBar);
    icon->setStyleSheet(QStringLiteral("background:#6366F1;padding:6px;border-radius:7px;"));
    titleLayout->addWidget(icon);
    auto* title = new QLabel(QStringLiteral("审计日志"), titleBar);
    title->setObjectName(QStringLiteral("appTitle"));
    titleLayout->addWidget(title);
    titleLayout->addStretch();
    exportButton_ = new QPushButton(QStringLiteral("⇩"), titleBar);
    exportButton_->setObjectName(QStringLiteral("iconButton"));
    exportButton_->setToolTip(QStringLiteral("导出已选择日志"));
    exportButton_->setEnabled(false);
    auto* refreshButton = new QPushButton(QStringLiteral("↻"), titleBar);
    refreshButton->setObjectName(QStringLiteral("iconButton"));
    refreshButton->setToolTip(QStringLiteral("刷新"));
    auto* clearButton = new QPushButton(QStringLiteral("⌫"), titleBar);
    clearButton->setObjectName(QStringLiteral("iconButton"));
    clearButton->setToolTip(QStringLiteral("清空全部"));
    titleLayout->addWidget(exportButton_);
    titleLayout->addWidget(refreshButton);
    titleLayout->addWidget(clearButton);
    connect(refreshButton, &QPushButton::clicked, this, &AuditLogWindow::refresh);
    connect(exportButton_, &QPushButton::clicked, this, [this]() {
        const QString path = QFileDialog::getSaveFileName(this, QStringLiteral("选择导出位置"), QStringLiteral("audit_logs.md"), QStringLiteral("Markdown (*.md)"));
        if (path.isEmpty()) return;
        QSaveFile file(path);
        if (!file.open(QIODevice::WriteOnly | QIODevice::Text)) return;
        QList<AuditLogModel> selected = selectedLogs_.values();
        std::sort(selected.begin(), selected.end(), [](const AuditLogModel& left, const AuditLogModel& right) {
            return left.timestamp > right.timestamp;
        });
        file.write(exportMarkdown(selected));
        if (!file.commit()) QMessageBox::warning(this, QStringLiteral("导出失败"), file.errorString());
    });
    connect(clearButton, &QPushButton::clicked, this, [this]() {
        if (QMessageBox::question(this, QStringLiteral("清空所有日志"), QStringLiteral("确定要清空所有审计日志吗？此操作无法撤销。")) != QMessageBox::Yes) return;
        if (bridge_ == nullptr) return;
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
            const QJsonObject response = watcher->result();
            watcher->deleteLater();
            if (!response.value(QStringLiteral("success")).toBool()) {
                QMessageBox::warning(this, QStringLiteral("清空失败"), response.value(QStringLiteral("error")).toString());
                return;
            }
            selectedLogs_.clear();
            resetAndRefresh();
        });
        watcher->setFuture(QtConcurrent::run([bridge = bridge_]() { return bridge->call("ClearAllAuditLogsFFI"); }));
    });
    root->addWidget(titleBar);

    auto* stats = new QWidget(shell);
    stats->setFixedHeight(40);
    auto* statsLayout = new QHBoxLayout(stats);
    statsLayout->setContentsMargins(16, 0, 16, 0);
    totalLabel_ = new QLabel(QStringLiteral("● 日志总数: 0"), stats);
    totalLabel_->setStyleSheet(QStringLiteral("color:#818CF8;"));
    riskLabel_ = new QLabel(QStringLiteral("● 风险次数: 0"), stats);
    riskLabel_->setStyleSheet(QStringLiteral("color:#F59E0B;"));
    blockedLabel_ = new QLabel(QStringLiteral("● 拦截次数: 0"), stats);
    blockedLabel_->setStyleSheet(QStringLiteral("color:#F87171;"));
    allowedLabel_ = new QLabel(QStringLiteral("● 允许次数: 0"), stats);
    allowedLabel_->setStyleSheet(QStringLiteral("color:#22C55E;"));
    statsLayout->addWidget(totalLabel_);
    statsLayout->addWidget(riskLabel_);
    statsLayout->addWidget(blockedLabel_);
    statsLayout->addWidget(allowedLabel_);
    statsLayout->addStretch();
    root->addWidget(stats);

    auto* content = new QWidget(shell);
    auto* contentLayout = new QVBoxLayout(content);
    contentLayout->setContentsMargins(12, 10, 12, 10);
    contentLayout->setSpacing(10);
    assetTabs_ = new QHBoxLayout;
    assetGroup_ = new QButtonGroup(this);
    assetGroup_->setExclusive(true);
    auto* allBots = new QPushButton(QStringLiteral("✓  全部 Bot"), content);
    allBots->setCheckable(true);
    allBots->setChecked(true);
    allBots->setObjectName(QStringLiteral("tabButton"));
    assetGroup_->addButton(allBots);
    connect(allBots, &QPushButton::clicked, this, [this]() { selectedAssetName_.clear(); selectedAssetId_.clear(); resetAndRefresh(); });
    assetTabs_->addWidget(allBots);
    assetTabs_->addStretch();
    contentLayout->addLayout(assetTabs_);
    auto* toolbar = new QHBoxLayout;
    search_ = new QLineEdit(content);
    search_->setPlaceholderText(QStringLiteral("搜索请求、回复、风险说明与消息/工具 JSON... 按回车搜索"));
    riskOnly_ = new QPushButton(QStringLiteral("仅显示风险"), content);
    riskOnly_->setCheckable(true);
    toolbar->addWidget(search_, 1);
    toolbar->addWidget(riskOnly_);
    contentLayout->addLayout(toolbar);
    connect(search_, &QLineEdit::returnPressed, this, &AuditLogWindow::resetAndRefresh);
    connect(riskOnly_, &QPushButton::toggled, this, [this]() { resetAndRefresh(); });
    contentSplitter_ = new QSplitter(Qt::Horizontal, content);
    contentSplitter_->setChildrenCollapsible(false);
    list_ = new QListWidget(contentSplitter_);
    list_->setSpacing(8);
    contentSplitter_->addWidget(list_);
    detailPanel_ = new QFrame(contentSplitter_);
    detailPanel_->setObjectName(QStringLiteral("auditDetailPanel"));
    detailPanel_->setMinimumWidth(500);
    auto* detailLayout = new QVBoxLayout(detailPanel_);
    detailLayout->setContentsMargins(16, 14, 16, 14);
    detailLayout->setSpacing(10);
    auto* detailHeader = new QHBoxLayout;
    detailTitle_ = new QLabel(QStringLiteral("日志详情"), detailPanel_);
    detailTitle_->setObjectName(QStringLiteral("dialogTitle"));
    detailHeader->addWidget(detailTitle_, 1);
    auto* closeDetail = new QPushButton(QStringLiteral("×"), detailPanel_);
    closeDetail->setObjectName(QStringLiteral("dialogCloseButton"));
    connect(closeDetail, &QPushButton::clicked, this, [this]() { detailPanel_->hide(); });
    detailHeader->addWidget(closeDetail);
    detailLayout->addLayout(detailHeader);
    auto* detailScroll = new QScrollArea(detailPanel_);
    detailScroll->setObjectName(QStringLiteral("auditDetailScroll"));
    detailScroll->setWidgetResizable(true);
    detailScroll->setFrameShape(QFrame::NoFrame);
    detailScroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    detailView_ = new AuditTimelineWidget(detailScroll);
    detailScroll->setWidget(detailView_);
    detailLayout->addWidget(detailScroll, 1);
    contentSplitter_->addWidget(detailPanel_);
    detailPanel_->hide();
    contentSplitter_->setStretchFactor(0, 2);
    contentSplitter_->setStretchFactor(1, 3);
    contentLayout->addWidget(contentSplitter_, 1);
    auto* pagination = new QHBoxLayout;
    pagination->addStretch();
    previousPageButton_ = new QPushButton(QStringLiteral("上一页"), content);
    pageLabel_ = new QLabel(QStringLiteral("第 1 / 1 页"), content);
    pageLabel_->setObjectName(QStringLiteral("subtle"));
    pageLabel_->setMinimumWidth(120);
    pageLabel_->setAlignment(Qt::AlignCenter);
    nextPageButton_ = new QPushButton(QStringLiteral("下一页"), content);
    pagination->addWidget(previousPageButton_);
    pagination->addWidget(pageLabel_);
    pagination->addWidget(nextPageButton_);
    pagination->addStretch();
    contentLayout->addLayout(pagination);
    connect(previousPageButton_, &QPushButton::clicked, this, [this]() {
        if (currentPage_ <= 0) return;
        --currentPage_;
        refresh();
    });
    connect(nextPageButton_, &QPushButton::clicked, this, [this]() {
        if ((currentPage_ + 1) * pageSize_ >= totalCount_) return;
        ++currentPage_;
        refresh();
    });
    connect(list_, &QListWidget::itemDoubleClicked, this, [this](QListWidgetItem* item) {
        const int index = item->data(Qt::UserRole).toInt();
        if (index >= 0 && index < logs_.size()) showDetail(logs_.at(index));
    });
    root->addWidget(content, 1);
    setCentralWidget(shell);
}

void AuditLogWindow::loadAssetTabs() {
    if (bridge_ == nullptr || !bridge_->isReady()) return;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonArray assets = watcher->result().value(QStringLiteral("data")).toArray();
        for (const QJsonValue& value : assets) {
            const QJsonObject asset = value.toObject();
            const QString name = asset.value(QStringLiteral("asset_name")).toString();
            const QString id = asset.value(QStringLiteral("asset_id")).toString();
            auto* button = new QPushButton(name.isEmpty() ? id : name, this);
            button->setCheckable(true);
            button->setObjectName(QStringLiteral("tabButton"));
            assetGroup_->addButton(button);
            assetTabs_->insertWidget(assetTabs_->count() - 1, button);
            connect(button, &QPushButton::clicked, this, [this, name, id]() { selectedAssetName_ = name; selectedAssetId_ = id; resetAndRefresh(); });
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_]() { return bridge->call("GetAuditLogAssetsFFI"); }));
}

void AuditLogWindow::refresh() {
    if (bridge_ == nullptr || !bridge_->isReady()) return;
    list_->clear();
    list_->addItem(QStringLiteral("正在加载审计日志..."));
    previousPageButton_->setEnabled(false);
    nextPageButton_->setEnabled(false);
    const AuditQuery query{currentPage_, pageSize_, riskOnly_->isChecked(), search_->text().trimmed(), selectedAssetId_};
    const quint64 generation = ++refreshGeneration_;
    auto* watcher = new QFutureWatcher<AuditPageResult>(this);
    connect(watcher, &QFutureWatcher<AuditPageResult>::finished, this, [this, watcher, generation]() {
        const AuditPageResult result = watcher->result();
        watcher->deleteLater();
        if (generation != refreshGeneration_) return;
        if (!result.success) {
            list_->clear();
            list_->addItem(QStringLiteral("加载失败: %1").arg(result.error));
            updatePagination(0);
            return;
        }
        const int pageCount = qMax(1, (result.statistics.total + pageSize_ - 1) / pageSize_);
        if (currentPage_ >= pageCount && currentPage_ > 0) {
            currentPage_ = pageCount - 1;
            refresh();
            return;
        }
        render(result);
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, query]() { return AuditService::fetchPage(*bridge, query); }));
}

void AuditLogWindow::resetAndRefresh() {
    currentPage_ = 0;
    refresh();
}

void AuditLogWindow::render(const AuditPageResult& page) {
    logs_ = page.logs;
    list_->clear();
    exportButton_->setEnabled(!selectedLogs_.isEmpty());
    for (int i = 0; i < page.logs.size(); ++i) {
        const AuditLogModel& log = page.logs.at(i);
        const QString normalizedAction = log.action.trimmed().toUpper();
        const bool blocked = normalizedAction == QStringLiteral("BLOCK") || normalizedAction == QStringLiteral("HARD_BLOCK");
        const QString action = normalizedAction == QStringLiteral("ALLOW") ? QStringLiteral("已允许")
            : blocked ? QStringLiteral("已拦截")
            : normalizedAction == QStringLiteral("WARN") ? QStringLiteral("有风险")
            : normalizedAction == QStringLiteral("NEEDS_CONFIRMATION") ? QStringLiteral("需确认") : log.action;
        auto* item = new QListWidgetItem(list_);
        item->setData(Qt::UserRole, i);
        item->setSizeHint(QSize(100, 100));
        item->setToolTip(QStringLiteral("双击查看日志详情"));
        auto* row = new QWidget(list_);
        auto* rowLayout = new QHBoxLayout(row);
        rowLayout->setContentsMargins(0, 0, 0, 0);
        rowLayout->setSpacing(8);
        auto* serial = new QPushButton(QString::number(qMax(1, page.statistics.total - currentPage_ * pageSize_ - i)), row);
        serial->setObjectName(QStringLiteral("auditSerialButton"));
        serial->setCheckable(true);
        serial->setChecked(selectedLogs_.contains(log.id));
        serial->setFixedSize(30, 30);
        serial->setToolTip(QStringLiteral("选择此日志用于导出"));
        connect(serial, &QPushButton::toggled, this, [this, log](bool checked) {
            if (checked) selectedLogs_.insert(log.id, log); else selectedLogs_.remove(log.id);
            exportButton_->setEnabled(!selectedLogs_.isEmpty());
        });
        rowLayout->addWidget(serial, 0, Qt::AlignVCenter);
        auto* card = new QPushButton(row);
        card->setObjectName(QStringLiteral("auditLogCard"));
        card->setCursor(Qt::PointingHandCursor);
        card->setMinimumHeight(84);
        auto* cardLayout = new QVBoxLayout(card);
        cardLayout->setContentsMargins(12, 10, 12, 10);
        cardLayout->setSpacing(7);
        auto* top = new QHBoxLayout;
        auto* actionBadge = new QLabel(action, card);
        actionBadge->setObjectName(blocked ? QStringLiteral("dangerPill") : log.hasRisk ? QStringLiteral("warningPill") : QStringLiteral("successPill"));
        top->addWidget(actionBadge);
        if (log.hasRisk && !log.riskLevel.isEmpty()) {
            const QString riskText = log.riskLevel.compare(QStringLiteral("HIGH"), Qt::CaseInsensitive) == 0 ? QStringLiteral("高")
                : log.riskLevel.compare(QStringLiteral("MEDIUM"), Qt::CaseInsensitive) == 0 ? QStringLiteral("中")
                : log.riskLevel.compare(QStringLiteral("LOW"), Qt::CaseInsensitive) == 0 ? QStringLiteral("低") : log.riskLevel;
            auto* riskBadge = new QLabel(riskText, card);
            riskBadge->setObjectName(QStringLiteral("warningPill"));
            top->addWidget(riskBadge);
        }
        top->addStretch();
        const QDateTime parsedTimestamp = QDateTime::fromString(log.timestamp, Qt::ISODateWithMs);
        auto* timestamp = new QLabel(parsedTimestamp.isValid() ? parsedTimestamp.toLocalTime().toString(QStringLiteral("yyyy-MM-dd HH:mm:ss")) : log.timestamp, card);
        timestamp->setObjectName(QStringLiteral("subtle"));
        top->addWidget(timestamp);
        auto* copy = new QPushButton(QStringLiteral("▣"), card);
        copy->setObjectName(QStringLiteral("auditCopyButton"));
        copy->setToolTip(QStringLiteral("复制请求内容"));
        connect(copy, &QPushButton::clicked, this, [content = log.requestContent]() { QApplication::clipboard()->setText(content); });
        top->addWidget(copy);
        cardLayout->addLayout(top);
        auto* content = new QLabel(log.requestContent.left(220), card);
        content->setObjectName(QStringLiteral("muted"));
        content->setWordWrap(true);
        content->setMaximumHeight(38);
        cardLayout->addWidget(content);
        connect(card, &QPushButton::clicked, this, [this, log]() { showDetail(log); });
        rowLayout->addWidget(card, 1);
        list_->setItemWidget(item, row);
    }
    if (page.logs.isEmpty()) list_->addItem(QStringLiteral("暂无审计日志"));
    totalLabel_->setText(QStringLiteral("● 日志总数: %1").arg(page.statistics.total));
    riskLabel_->setText(QStringLiteral("● 风险次数: %1").arg(page.statistics.riskCount));
    blockedLabel_->setText(QStringLiteral("● 拦截次数: %1").arg(page.statistics.blockedCount));
    allowedLabel_->setText(QStringLiteral("● 允许次数: %1").arg(page.statistics.allowedCount));
    updatePagination(page.statistics.total);
}

void AuditLogWindow::updatePagination(int total) {
    totalCount_ = qMax(0, total);
    const int pageCount = qMax(1, (totalCount_ + pageSize_ - 1) / pageSize_);
    pageLabel_->setText(QStringLiteral("第 %1 / %2 页 · 共 %3 条").arg(currentPage_ + 1).arg(pageCount).arg(totalCount_));
    previousPageButton_->setEnabled(currentPage_ > 0);
    nextPageButton_->setEnabled((currentPage_ + 1) * pageSize_ < totalCount_);
}

void AuditLogWindow::showDetail(const AuditLogModel& log) {
    const QString baseTitle = QStringLiteral("日志详情 · %1").arg(log.assetName.isEmpty() ? QStringLiteral("Bot") : log.assetName);
    detailTitle_->setText(baseTitle + QStringLiteral(" · 正在加载关联事件…"));
    detailView_->setLog(log);
    detailPanel_->show();
    contentSplitter_->setSizes({440, 720});
    const quint64 generation = ++detailGeneration_;
    if (log.requestId.trimmed().isEmpty()) {
        detailTitle_->setText(baseTitle);
        return;
    }

    using RelatedEventsResult = QPair<QList<SecurityEventModel>, QString>;
    auto* watcher = new QFutureWatcher<RelatedEventsResult>(this);
    connect(watcher, &QFutureWatcher<RelatedEventsResult>::finished, this,
            [this, watcher, generation, log, baseTitle]() {
        const RelatedEventsResult result = watcher->result();
        watcher->deleteLater();
        if (generation != detailGeneration_) return;
        detailTitle_->setText(baseTitle);
        detailTitle_->setToolTip(result.second);
        detailView_->setLog(log, result.first);
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, log]() {
        QString error;
        const QList<SecurityEventModel> events = AuditService::fetchRelatedEvents(*bridge, log, &error);
        return qMakePair(events, error);
    }));
}
