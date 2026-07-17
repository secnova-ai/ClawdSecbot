#include "ui/dialogs/SkillScanDialog.h"

#include "bridge/GoBridge.h"

#include <QDialogButtonBox>
#include <QFrame>
#include <QFutureWatcher>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonObject>
#include <QLabel>
#include <QMessageBox>
#include <QPlainTextEdit>
#include <QProgressBar>
#include <QPushButton>
#include <QScrollArea>
#include <QStackedWidget>
#include <QStyle>
#include <QTimer>
#include <QVBoxLayout>
#include <QtConcurrent>

namespace {
QLabel* scanLabel(const QString& text, const char* name, QWidget* parent) {
    auto* label = new QLabel(text, parent);
    label->setObjectName(QString::fromLatin1(name));
    return label;
}

void clearLayout(QLayout* layout) {
    while (layout != nullptr && layout->count() > 0) {
        QLayoutItem* item = layout->takeAt(0);
        if (item->widget() != nullptr) item->widget()->deleteLater();
        if (item->layout() != nullptr) clearLayout(item->layout());
        delete item;
    }
}

QString riskText(const QString& level) {
    if (level.compare(QStringLiteral("critical"), Qt::CaseInsensitive) == 0) return QStringLiteral("严重风险");
    if (level.compare(QStringLiteral("high"), Qt::CaseInsensitive) == 0) return QStringLiteral("高风险");
    if (level.compare(QStringLiteral("medium"), Qt::CaseInsensitive) == 0) return QStringLiteral("中风险");
    if (level.compare(QStringLiteral("low"), Qt::CaseInsensitive) == 0) return QStringLiteral("低风险");
    return QStringLiteral("检测到风险");
}
}

SkillScanDialog::SkillScanDialog(GoBridge* bridge, const QString& assetName, QWidget* parent)
    : QDialog(parent), bridge_(bridge), assetName_(assetName) {
    setWindowFlags(Qt::Dialog | Qt::FramelessWindowHint);
    setWindowTitle(QStringLiteral("AI 技能安全分析"));
    resize(760, 620);
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(24, 24, 24, 20);
    root->setSpacing(14);
    auto* header = new QHBoxLayout;
    auto* icon = scanLabel(QStringLiteral("⌕"), "dialogHeaderIcon", this);
    icon->setAlignment(Qt::AlignCenter);
    icon->setFixedSize(36, 36);
    header->addWidget(icon);
    header->addWidget(scanLabel(QStringLiteral("AI 技能安全分析"), "dialogTitle", this), 1);
    auto* close = new QPushButton(QStringLiteral("×"), this);
    close->setObjectName(QStringLiteral("dialogCloseButton"));
    header->addWidget(close);
    root->addLayout(header);

    status_ = scanLabel(QStringLiteral("正在发现并扫描 Skill 中的提示词注入、数据窃取、代码执行和供应链风险…"), "muted", this);
    status_->setWordWrap(true);
    root->addWidget(status_);
    progress_ = new QProgressBar(this);
    progress_->setRange(0, 0);
    progress_->setTextVisible(false);
    root->addWidget(progress_);

    pages_ = new QStackedWidget(this);
    logView_ = new QPlainTextEdit(pages_);
    logView_->setObjectName(QStringLiteral("skillScanLog"));
    logView_->setReadOnly(true);
    logView_->setPlaceholderText(QStringLiteral("正在等待扫描日志…"));
    pages_->addWidget(logView_);
    resultsContent_ = new QWidget(pages_);
    resultsLayout_ = new QVBoxLayout(resultsContent_);
    resultsLayout_->setContentsMargins(0, 0, 0, 0);
    resultsLayout_->setSpacing(12);
    resultsScroll_ = new QScrollArea(pages_);
    resultsScroll_->setWidgetResizable(true);
    resultsScroll_->setFrameShape(QFrame::NoFrame);
    resultsScroll_->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    resultsScroll_->setWidget(resultsContent_);
    pages_->addWidget(resultsScroll_);
    root->addWidget(pages_, 1);

    closeButton_ = new QPushButton(QStringLiteral("取消"), this);
    root->addWidget(closeButton_, 0, Qt::AlignRight);
    pollTimer_ = new QTimer(this);
    pollTimer_->setInterval(250);
    connect(pollTimer_, &QTimer::timeout, this, &SkillScanDialog::pollBatch);
    auto closeDialog = [this]() {
        pollTimer_->stop();
        if (!batchId_.isEmpty() && bridge_ != nullptr) {
            const QString batch = batchId_;
            const QString asset = assetName_;
            [[maybe_unused]] const auto cancelFuture = QtConcurrent::run([bridge = bridge_, batch, asset]() {
                return asset.isEmpty() ? bridge->call("CancelBatchSkillScan", batch)
                                       : bridge->call("CancelBatchSkillScanByAssetFFI", asset, batch);
            });
        }
        reject();
    };
    connect(close, &QPushButton::clicked, this, closeDialog);
    connect(closeButton_, &QPushButton::clicked, this, [this, closeDialog]() {
        if (batchId_.isEmpty()) accept(); else closeDialog();
    });
    QTimer::singleShot(0, this, &SkillScanDialog::startScan);
}

void SkillScanDialog::startScan() {
    if (bridge_ == nullptr || !bridge_->isReady()) return finishWithError(QStringLiteral("Go 业务库尚未就绪"));
    const QString asset = assetName_;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject response = watcher->result();
        watcher->deleteLater();
        if (!response.value(QStringLiteral("success")).toBool()) return finishWithError(response.value(QStringLiteral("error")).toString(QStringLiteral("启动扫描失败")));
        const int total = response.value(QStringLiteral("total")).toInt();
        batchId_ = response.value(QStringLiteral("batch_id")).toString();
        if (total == 0 || batchId_.isEmpty()) {
            progress_->setRange(0, 1);
            progress_->setValue(1);
            status_->setText(QStringLiteral("没有发现需要扫描的 Skill"));
            closeButton_->setText(QStringLiteral("完成"));
            closeButton_->setObjectName(QStringLiteral("primaryButton"));
            closeButton_->style()->unpolish(closeButton_);
            closeButton_->style()->polish(closeButton_);
            pages_->setCurrentWidget(resultsScroll_);
            resultsLayout_->addWidget(scanLabel(QStringLiteral("⌕\n\n没有需要扫描的 Skill"), "muted", resultsContent_), 0, Qt::AlignCenter);
            resultsLayout_->addStretch();
            return;
        }
        progress_->setRange(0, total);
        progress_->setValue(0);
        status_->setText(QStringLiteral("已发现 %1 个 Skill，正在进行安全分析…").arg(total));
        pollTimer_->start();
        pollBatch();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, asset]() {
        return asset.isEmpty() ? bridge->call("StartBatchSkillScan") : bridge->call("StartBatchSkillScanByAssetFFI", asset);
    }));
}

void SkillScanDialog::pollBatch() {
    if (polling_ || batchId_.isEmpty() || bridge_ == nullptr) return;
    polling_ = true;
    const QString batch = batchId_;
    const QString asset = assetName_;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        polling_ = false;
        const QJsonObject response = watcher->result();
        watcher->deleteLater();
        for (const QJsonValue& value : response.value(QStringLiteral("logs")).toArray()) logView_->appendPlainText(value.toString());
        const int total = response.value(QStringLiteral("total")).toInt(progress_->maximum());
        const int current = response.value(QStringLiteral("current_index")).toInt();
        if (total > 0) progress_->setRange(0, total);
        progress_->setValue(current);
        const QString skill = response.value(QStringLiteral("current_skill")).toString();
        status_->setText(skill.isEmpty() ? QStringLiteral("正在扫描 %1 / %2").arg(current).arg(total)
                                         : QStringLiteral("正在扫描 %1 / %2：%3").arg(current).arg(total).arg(skill));
        if (response.value(QStringLiteral("completed")).toBool()) {
            pollTimer_->stop();
            if (!response.value(QStringLiteral("error")).toString().isEmpty()) return finishWithError(response.value(QStringLiteral("error")).toString());
            status_->setText(QStringLiteral("扫描完成，正在加载结果…"));
            loadResults();
        }
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, batch, asset]() {
        return asset.isEmpty() ? bridge->call("GetBatchSkillScanLog", batch)
                               : bridge->call("GetBatchSkillScanLogByAssetFFI", asset, batch);
    }));
}

void SkillScanDialog::loadResults() {
    const QString batch = batchId_;
    const QString asset = assetName_;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject response = watcher->result();
        watcher->deleteLater();
        if (!response.value(QStringLiteral("success")).toBool()) return finishWithError(response.value(QStringLiteral("error")).toString(QStringLiteral("加载扫描结果失败")));
        renderResults(response);
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, batch, asset]() {
        return asset.isEmpty() ? bridge->call("GetBatchSkillScanResults", batch)
                               : bridge->call("GetBatchSkillScanResultsByAssetFFI", asset, batch);
    }));
}

void SkillScanDialog::renderResults(const QJsonObject& response) {
    clearLayout(resultsLayout_);
    const QJsonObject results = response.value(QStringLiteral("results")).toObject();
    int riskCount = 0;
    for (auto it = results.constBegin(); it != results.constEnd(); ++it) {
        const QJsonObject item = it.value().toObject();
        const QJsonObject result = item.value(QStringLiteral("result")).toObject();
        const bool succeeded = item.value(QStringLiteral("success")).toBool();
        const bool safe = succeeded && result.value(QStringLiteral("safe")).toBool();
        if (!safe) ++riskCount;
        auto* card = new QFrame(resultsContent_);
        card->setObjectName(succeeded ? (safe ? QStringLiteral("skillSafeCard") : QStringLiteral("skillRiskCard")) : QStringLiteral("skillFailedCard"));
        auto* layout = new QVBoxLayout(card);
        layout->setContentsMargins(14, 12, 14, 12);
        layout->setSpacing(6);
        auto* top = new QHBoxLayout;
        top->addWidget(scanLabel(safe ? QStringLiteral("✓") : QStringLiteral("△"), safe ? "skillSafeIcon" : "skillRiskIcon", card));
        top->addWidget(scanLabel(item.value(QStringLiteral("skill_name")).toString(it.key()), "sectionTitle", card), 1);
        top->addWidget(scanLabel(safe ? QStringLiteral("安全") : succeeded ? riskText(result.value(QStringLiteral("risk_level")).toString()) : QStringLiteral("扫描失败"),
                                 safe ? "successPill" : "dangerPill", card));
        layout->addLayout(top);
        const QString path = item.value(QStringLiteral("skill_path")).toString();
        if (!path.isEmpty()) layout->addWidget(scanLabel(QStringLiteral("▱  %1").arg(path), "skillPath", card));
        const QString summary = succeeded ? result.value(QStringLiteral("summary")).toString() : item.value(QStringLiteral("error")).toString();
        if (!summary.isEmpty()) {
            auto* detail = scanLabel(summary, "muted", card);
            detail->setWordWrap(true);
            layout->addWidget(detail);
        }
        const QJsonArray issues = result.value(QStringLiteral("issues")).toArray();
        for (const QJsonValue& issueValue : issues) {
            const QJsonObject issue = issueValue.toObject();
            QString issueText = issueValue.isString() ? issueValue.toString() : issue.value(QStringLiteral("description")).toString(issue.value(QStringLiteral("title")).toString());
            auto* issueLabel = scanLabel(QStringLiteral("- %1").arg(issueText), "subtle", card);
            issueLabel->setWordWrap(true);
            layout->addWidget(issueLabel);
        }
        if (succeeded && !safe) {
            auto* actions = new QHBoxLayout;
            actions->addStretch();
            auto* trust = new QPushButton(QStringLiteral("标记为可信"), card);
            auto* remove = new QPushButton(QStringLiteral("删除 Skill"), card);
            remove->setObjectName(QStringLiteral("dangerButton"));
            actions->addWidget(trust);
            actions->addWidget(remove);
            layout->addLayout(actions);
            const QString hash = item.value(QStringLiteral("skill_hash")).toString();
            connect(trust, &QPushButton::clicked, card, [this, trust, hash]() {
                trust->setEnabled(false);
                auto* watcher = new QFutureWatcher<QJsonObject>(trust);
                connect(watcher, &QFutureWatcher<QJsonObject>::finished, trust, [watcher, trust]() {
                    trust->setText(watcher->result().value(QStringLiteral("success")).toBool() ? QStringLiteral("已信任") : QStringLiteral("信任失败"));
                    watcher->deleteLater();
                });
                watcher->setFuture(QtConcurrent::run([bridge = bridge_, hash]() { return bridge->call("TrustSkillScan", hash); }));
            });
            connect(remove, &QPushButton::clicked, card, [this, remove, path, hash]() {
                if (QMessageBox::warning(this, QStringLiteral("删除 Skill"), QStringLiteral("确定要删除此 Skill 吗？此操作会移动或删除其目录。"),
                                         QMessageBox::Cancel | QMessageBox::Yes, QMessageBox::Cancel) != QMessageBox::Yes) return;
                remove->setEnabled(false);
                auto* watcher = new QFutureWatcher<QJsonObject>(remove);
                connect(watcher, &QFutureWatcher<QJsonObject>::finished, remove, [this, watcher, remove, hash]() {
                    if (watcher->result().value(QStringLiteral("success")).toBool()) {
                        [[maybe_unused]] const auto deleteRecordFuture = QtConcurrent::run([bridge = bridge_, hash]() { return bridge->call("DeleteSkillScanFFI", hash); });
                        remove->setText(QStringLiteral("已删除"));
                    } else remove->setText(QStringLiteral("删除失败"));
                    watcher->deleteLater();
                });
                watcher->setFuture(QtConcurrent::run([bridge = bridge_, path]() { return bridge->call("DeleteSkill", path); }));
            });
        }
        resultsLayout_->addWidget(card);
    }
    if (results.isEmpty()) resultsLayout_->addWidget(scanLabel(QStringLiteral("没有返回扫描结果"), "muted", resultsContent_), 0, Qt::AlignCenter);
    resultsLayout_->addStretch();
    pages_->setCurrentWidget(resultsScroll_);
    progress_->setValue(progress_->maximum());
    status_->setText(riskCount == 0 ? QStringLiteral("扫描完成：未发现风险") : QStringLiteral("扫描完成：%1 个 Skill 需要处理").arg(riskCount));
    batchId_.clear();
    closeButton_->setText(QStringLiteral("完成"));
    closeButton_->setObjectName(QStringLiteral("primaryButton"));
    closeButton_->style()->unpolish(closeButton_);
    closeButton_->style()->polish(closeButton_);
}

void SkillScanDialog::finishWithError(const QString& error) {
    pollTimer_->stop();
    progress_->setRange(0, 1);
    progress_->setValue(0);
    status_->setText(QStringLiteral("扫描失败：%1").arg(error));
    logView_->appendPlainText(error);
    batchId_.clear();
    closeButton_->setText(QStringLiteral("关闭"));
}
