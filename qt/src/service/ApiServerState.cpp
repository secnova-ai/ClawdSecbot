#include "service/ApiServerState.h"

namespace ApiServerState {
bool isAlreadyInDesiredState(const QJsonObject& result, bool enabled) {
    const QString error = result.value(QStringLiteral("error")).toString();
    return enabled ? error == QStringLiteral("API server is already running")
                   : error == QStringLiteral("API server is not running");
}
}
