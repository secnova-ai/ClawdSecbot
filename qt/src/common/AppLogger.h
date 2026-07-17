#pragma once

#include <QString>

class AppLogger {
public:
    static void initialize(const QString& logDir, bool verbose);
    static void info(const QString& message);
    static void error(const QString& message);
    static void shutdown();
};
