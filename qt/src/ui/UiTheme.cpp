#include "ui/UiTheme.h"

#include <QApplication>
#include <QFile>
#include <QSettings>

namespace {
constexpr auto kThemeSetting = "ui/theme";
constexpr auto kDefaultThemeId = "light";

QString themeResourcePath(const QString& themeId) {
    return QStringLiteral(":/themes/%1.qss").arg(themeId.isEmpty() ? QString::fromLatin1(kDefaultThemeId) : themeId);
}
}

QString UiTheme::defaultThemeId() {
    return QString::fromLatin1(kDefaultThemeId);
}

QString UiTheme::currentThemeId() {
    const QString value = QSettings{}.value(QString::fromLatin1(kThemeSetting), defaultThemeId()).toString().trimmed().toLower();
    return value.isEmpty() ? defaultThemeId() : value;
}

bool UiTheme::applyCurrentTheme(QApplication* app, QString* errorMessage) {
    return applyTheme(app, currentThemeId(), errorMessage);
}

bool UiTheme::applyTheme(QApplication* app, const QString& themeId, QString* errorMessage) {
    if (app == nullptr) {
        if (errorMessage != nullptr) {
            *errorMessage = QStringLiteral("QApplication is unavailable.");
        }
        return false;
    }
    app->setStyleSheet(styleSheet(themeId, errorMessage));
    return true;
}

QString UiTheme::styleSheet(const QString& themeId, QString* errorMessage) {
    QFile file(themeResourcePath(themeId));
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text)) {
        if (errorMessage != nullptr) {
            *errorMessage = QStringLiteral("Failed to open theme resource: %1").arg(file.fileName());
        }
        return {};
    }
    return QString::fromUtf8(file.readAll());
}
