#include "common/AppLogger.h"

#include <QDir>
#include <QDateTime>
#include <QFile>
#include <QLoggingCategory>
#include <QMutex>
#include <QTextStream>

namespace {
QFile logFile;
QMutex logMutex;

void writeMessage(const QString& level, const QString& message) {
    QMutexLocker locker(&logMutex);
    if (!logFile.isOpen()) return;
    QTextStream stream(&logFile);
    stream << QDateTime::currentDateTime().toString(Qt::ISODateWithMs) << " [" << level << "] " << message << '\n';
    stream.flush();
}
}

void AppLogger::initialize(const QString& logDir, const bool verbose) {
    QDir{}.mkpath(logDir);
    logFile.setFileName(QDir(logDir).filePath(QStringLiteral("qt_app.log")));
    if (!logFile.open(QIODevice::WriteOnly | QIODevice::Append | QIODevice::Text)) return;
    if (!verbose) {
        QLoggingCategory::setFilterRules(QStringLiteral("qt.*=false"));
    }
}

void AppLogger::info(const QString& message) { writeMessage(QStringLiteral("INFO"), message); }
void AppLogger::error(const QString& message) { writeMessage(QStringLiteral("ERROR"), message); }

void AppLogger::shutdown() {
    QMutexLocker locker(&logMutex);
    if (logFile.isOpen()) logFile.close();
}
