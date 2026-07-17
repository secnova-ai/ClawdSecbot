#include "app/AppController.h"
#include "common/AppLogger.h"
#include "core/AppConfig.h"
#include "ui/UiTheme.h"

#include <QApplication>
#include <QFontDatabase>
#include <QIcon>

int main(int argc, char* argv[]) {
    QApplication app(argc, argv);
    app.setQuitOnLastWindowClosed(false);
    QApplication::setApplicationName(QStringLiteral("clawdsecbot"));
    QApplication::setApplicationDisplayName(QStringLiteral("ClawdSecbot"));
    QApplication::setOrganizationName(QStringLiteral("ClawdSecbot"));
    QApplication::setOrganizationDomain(QStringLiteral("com.bot.secnova"));
    QApplication::setWindowIcon(QIcon(QStringLiteral(":/images/app_icon.png")));

    QFontDatabase::addApplicationFont(QStringLiteral(":/fonts/Inter-Regular.ttf"));
    QFontDatabase::addApplicationFont(QStringLiteral(":/fonts/Inter-Medium.ttf"));
    QFontDatabase::addApplicationFont(QStringLiteral(":/fonts/Inter-SemiBold.ttf"));
    QFontDatabase::addApplicationFont(QStringLiteral(":/fonts/Inter-Bold.ttf"));
    QFontDatabase::addApplicationFont(QStringLiteral(":/fonts/NotoSansSC-Regular.ttf"));
    QFontDatabase::addApplicationFont(QStringLiteral(":/fonts/NotoSansSC-Medium.ttf"));
    QFontDatabase::addApplicationFont(QStringLiteral(":/fonts/NotoSansSC-Bold.ttf"));

    UiTheme::applyCurrentTheme(&app);

    const AppConfig config = AppConfig::load();
    AppLogger::initialize(config.logDir, config.verboseLogging);

    int code = 0;
    {
        AppController controller(config);
        code = app.exec();
    }

    AppLogger::shutdown();
    return code;
}
