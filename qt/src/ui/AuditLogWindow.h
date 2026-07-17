#pragma once

#include "domain/Models.h"
#include "service/AuditService.h"

#include <QHash>
#include <QMainWindow>

class GoBridge;
class AuditTimelineWidget;
class QButtonGroup;
class QHBoxLayout;
class QLabel;
class QLineEdit;
class QListWidget;
class QPushButton;
class QSplitter;
class QWidget;

class AuditLogWindow final : public QMainWindow {
    Q_OBJECT
public:
    explicit AuditLogWindow(GoBridge* bridge, QWidget* parent = nullptr);

private:
    void buildUi();
    void loadAssetTabs();
    void refresh();
    void resetAndRefresh();
    void render(const AuditPageResult& page);
    void updatePagination(int total);
    void showDetail(const AuditLogModel& log);

    GoBridge* bridge_;
    QButtonGroup* assetGroup_ = nullptr;
    QHBoxLayout* assetTabs_ = nullptr;
    QString selectedAssetName_;
    QString selectedAssetId_;
    QLabel* totalLabel_ = nullptr;
    QLabel* riskLabel_ = nullptr;
    QLabel* blockedLabel_ = nullptr;
    QLabel* allowedLabel_ = nullptr;
    QLineEdit* search_ = nullptr;
    QPushButton* riskOnly_ = nullptr;
    QListWidget* list_ = nullptr;
    QSplitter* contentSplitter_ = nullptr;
    QWidget* detailPanel_ = nullptr;
    QLabel* detailTitle_ = nullptr;
    AuditTimelineWidget* detailView_ = nullptr;
    QPushButton* exportButton_ = nullptr;
    QPushButton* previousPageButton_ = nullptr;
    QPushButton* nextPageButton_ = nullptr;
    QLabel* pageLabel_ = nullptr;
    QList<AuditLogModel> logs_;
    QHash<QString, AuditLogModel> selectedLogs_;
    int currentPage_ = 0;
    int pageSize_ = 50;
    int totalCount_ = 0;
    quint64 refreshGeneration_ = 0;
    quint64 detailGeneration_ = 0;
};
