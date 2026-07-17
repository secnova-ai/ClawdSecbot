#pragma once

#include <QString>

class QApplication;

class UiTheme {
public:
    static QString defaultThemeId();
    static QString currentThemeId();
    static bool applyCurrentTheme(QApplication* app, QString* errorMessage = nullptr);
    static bool applyTheme(QApplication* app, const QString& themeId, QString* errorMessage = nullptr);
    static QString styleSheet(const QString& themeId = QString(), QString* errorMessage = nullptr);
};
