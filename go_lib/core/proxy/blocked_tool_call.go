package proxy

import (
	"strings"
	"time"
)

const blockedToolCallIDTTL = 2 * time.Hour

func normalizeBlockedToolCallID(toolCallID string) string {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(toolCallID))
	for _, r := range toolCallID {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') {
			if r >= 'A' && r <= 'Z' {
				r = r - 'A' + 'a'
			}
			b.WriteRune(r)
		}
	}
	matchID := strings.TrimSpace(b.String())
	if matchID == "" {
		return strings.ToLower(toolCallID)
	}
	return matchID
}

func (pp *ProxyProtection) markBlockedToolCallIDsForRequest(requestID string, toolCallIDs []string) {
	pp.markBlockedToolCallIDsForRequestAt(requestID, toolCallIDs, time.Now(), blockedToolCallIDTTL)
}

func (pp *ProxyProtection) markBlockedToolCallIDsForRequestAt(requestID string, toolCallIDs []string, now time.Time, ttl time.Duration) {
	if pp == nil || len(toolCallIDs) == 0 {
		return
	}
	if ttl <= 0 {
		ttl = blockedToolCallIDTTL
	}
	chainID := pp.chainIDForRequest(requestID)
	if chainID == "" {
		chain := pp.createSecurityChain(requestID, securityChainSourceTemp, "blocked_tool_call_without_chain")
		if chain != nil {
			chainID = chain.ChainID
		}
	}
	if chainID == "" {
		return
	}
	expiresAt := now.Add(ttl)
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chain := pp.chains[chainID]
	if chain == nil {
		return
	}
	marked := 0
	for _, rawID := range toolCallIDs {
		toolCallID := normalizeBlockedToolCallID(rawID)
		if toolCallID == "" {
			continue
		}
		chain.BlockedToolCallIDs[toolCallID] = expiresAt
		chain.ToolCallIDs[toolCallID] = struct{}{}
		if pp.toolCallToChains[toolCallID] == nil {
			pp.toolCallToChains[toolCallID] = make(map[string]securityChainBinding)
		}
		pp.toolCallToChains[toolCallID][chainID] = securityChainBinding{
			ChainID:   chainID,
			ExpiresAt: expiresAt,
		}
		marked++
	}
	if marked > 0 {
		chain.UpdatedAt = now
		chain.ExpiresAt = now.Add(securityChainTTL)
		logSecurityFlowInfo(
			securityFlowStageQuarantine,
			"blocked tool_call IDs quarantined: instruction_chain_id=%s request_id=%s count=%d expires_at=%s",
			chainID,
			strings.TrimSpace(requestID),
			marked,
			expiresAt.Format(time.RFC3339Nano),
		)
	}
}

func (pp *ProxyProtection) clearBlockedToolCallIDsForRequest(requestID string, toolCallIDs []string) int {
	return pp.clearBlockedToolCallIDsForRequestAt(requestID, toolCallIDs, time.Now())
}

func (pp *ProxyProtection) clearBlockedToolCallIDsForRequestAt(requestID string, toolCallIDs []string, now time.Time) int {
	if pp == nil || len(toolCallIDs) == 0 {
		return 0
	}
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	if chainID == "" {
		return 0
	}
	chain := pp.chains[chainID]
	if chain == nil {
		return 0
	}
	cleared := 0
	for _, rawID := range toolCallIDs {
		toolCallID := normalizeBlockedToolCallID(rawID)
		if toolCallID == "" {
			continue
		}
		if _, ok := chain.BlockedToolCallIDs[toolCallID]; ok {
			delete(chain.BlockedToolCallIDs, toolCallID)
			cleared++
		}
	}
	if cleared > 0 {
		chain.UpdatedAt = now
		chain.ExpiresAt = now.Add(securityChainTTL)
		logSecurityFlowInfo(securityFlowStageQuarantine, "blocked tool_call IDs cleared: instruction_chain_id=%s request_id=%s count=%d", chainID, strings.TrimSpace(requestID), cleared)
	}
	return cleared
}

func (pp *ProxyProtection) isBlockedToolCallIDForRequest(requestID, toolCallID string) bool {
	return pp.isBlockedToolCallIDForRequestAt(requestID, toolCallID, time.Now())
}

func (pp *ProxyProtection) isBlockedToolCallIDForRequestAt(requestID, toolCallID string, now time.Time) bool {
	if pp == nil {
		return false
	}
	toolCallID = normalizeBlockedToolCallID(toolCallID)
	if toolCallID == "" {
		return false
	}
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	if chainID != "" {
		chain := pp.chains[chainID]
		if chain == nil {
			return false
		}
		_, ok := chain.BlockedToolCallIDs[toolCallID]
		return ok
	}
	chainID, conflicted := pp.resolveUniqueSecurityChainByToolCallIDsLocked([]string{toolCallID}, now)
	if conflicted {
		logSecurityFlowWarning(securityFlowStageQuarantine, "blocked tool_call lookup ambiguous: request_id=%s tool_call_id=%s", strings.TrimSpace(requestID), toolCallID)
		return false
	}
	if chainID == "" {
		return false
	}
	chain := pp.chains[chainID]
	if chain == nil {
		return false
	}
	_, ok := chain.BlockedToolCallIDs[toolCallID]
	return ok
}

func (pp *ProxyProtection) isBlockedToolCallID(toolCallID string) bool {
	return pp.isBlockedToolCallIDForRequest("", toolCallID)
}
