#pragma once

#include <QJsonObject>

namespace ApiServerState {
bool isAlreadyInDesiredState(const QJsonObject& result, bool enabled);
}
