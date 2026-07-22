package coordinator

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/singu/proximity-unlock/internal/authorize"
	"github.com/singu/proximity-unlock/internal/ble"
	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/ipc"
	"github.com/singu/proximity-unlock/internal/protocol"
	"github.com/singu/proximity-unlock/internal/proximity"
	"github.com/singu/proximity-unlock/internal/secret"
	"github.com/singu/proximity-unlock/internal/windowskey"
)

type Signer interface {
	Sign([]byte) ([protocol.SignatureSize]byte, error)
	PublicKey() (*ecdsa.PublicKey, error)
}

type Coordinator struct {
	mu sync.Mutex

	configPath string
	config     config.Config
	secrets    secret.Store
	signer     Signer
	transport  ble.Transport
	state      *proximity.State
	authorizer *authorize.Manager
	replay     *protocol.ReplayGuard

	activeSession             uint32
	sessionActive             bool
	locked                    bool
	challengeBusy             bool
	lastChallenge             time.Time
	pairing                   *pendingPairing
	sessionValid              func(uint32) bool
	lastCPEvent               string
	lastCPEventAt             time.Time
	lastAuthFailureCode       string
	lastAuthFailureReason     string
	lastAuthFailureAt         time.Time
	authLogger                func(AuthenticationLogEntry)
	lastAuthOutcome           string
	predictionActive          bool
	predictionUnlockPending   bool
	predictionUnlockPendingAt time.Time
	predictionGuardActive     bool
	recentEvents              []RecentEvent
	nextEventID               uint64
}

type AuthenticationLogEntry struct {
	Warning bool
	Code    string
	Message string
}

const maxRecentEvents = 100

type RecentEvent struct {
	ID      uint64    `json:"id"`
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Detail  string    `json:"detail,omitempty"`
	Result  string    `json:"result"`
	Warning bool      `json:"warning"`
}

type authenticationError struct {
	code     string
	message  string
	security bool
	cause    error
}

func (e *authenticationError) Error() string { return e.message }
func (e *authenticationError) Unwrap() error { return e.cause }

func authError(code, message string, security bool, cause error) error {
	return &authenticationError{code: code, message: message, security: security, cause: cause}
}

func classifyAuthenticationError(err error) *authenticationError {
	var failure *authenticationError
	if errors.As(err, &failure) {
		return failure
	}
	return &authenticationError{code: "local_error", message: "电脑端认证处理失败", cause: err}
}

func (c *Coordinator) SetSessionValidator(validator func(uint32) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionValid = validator
}

func (c *Coordinator) SetAuthenticationLogger(logger func(AuthenticationLogEntry)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authLogger = logger
}

type pendingPairing struct {
	secret    [32]byte
	expiresAt time.Time
}

type Status struct {
	Configured            bool               `json:"configured"`
	Paired                bool               `json:"paired"`
	CredentialValid       bool               `json:"credential_valid"`
	Mode                  string             `json:"mode"`
	AutoLock              bool               `json:"auto_lock"`
	HighSensitivity       bool               `json:"high_sensitivity"`
	DopplerPrediction     bool               `json:"doppler_prediction"`
	DopplerSensitivity    int                `json:"doppler_sensitivity"`
	DopplerTriggered      bool               `json:"doppler_triggered"`
	PredictedRSSI         int                `json:"predicted_rssi,omitempty"`
	RSSISlopeDBPerSec     float64            `json:"rssi_slope_db_per_sec,omitempty"`
	ImmediateUnlock       bool               `json:"immediate_unlock"`
	FailureCooldown       bool               `json:"failure_cooldown_enabled"`
	PausedUntil           time.Time          `json:"paused_until,omitempty"`
	SessionActive         bool               `json:"session_active"`
	Locked                bool               `json:"locked"`
	MedianRSSI            int                `json:"median_rssi,omitempty"`
	HasRSSI               bool               `json:"has_rssi"`
	LastAuthenticated     time.Time          `json:"last_authenticated,omitempty"`
	ShouldLock            bool               `json:"should_lock"`
	BLEBackend            string             `json:"ble_backend"`
	Authorization         authorize.Snapshot `json:"authorization"`
	CooldownUntil         time.Time          `json:"cooldown_until,omitempty"`
	LastAuthFailureCode   string             `json:"last_authentication_failure_code,omitempty"`
	LastAuthFailureReason string             `json:"last_authentication_failure_reason,omitempty"`
	LastAuthFailureAt     time.Time          `json:"last_authentication_failure_at,omitempty"`
	LastCPEvent           string             `json:"last_credential_provider_event,omitempty"`
	LastCPEventAt         time.Time          `json:"last_credential_provider_event_at,omitempty"`
	UnlockRSSI            int                `json:"unlock_rssi"`
	LockRSSI              int                `json:"lock_rssi"`
	HighSensitivityRSSI   int                `json:"high_sensitivity_rssi"`
	RecentEvents          []RecentEvent      `json:"recent_events"`
}

func New(configPath string, cfg config.Config, secrets secret.Store, signer Signer, transport ble.Transport, authorizer *authorize.Manager) *Coordinator {
	settings := settingsFor(cfg)
	result := &Coordinator{
		configPath: configPath,
		config:     cfg,
		secrets:    secrets,
		signer:     signer,
		transport:  transport,
		state:      proximity.NewState(settings),
		authorizer: authorizer,
		replay:     protocol.NewReplayGuard(time.Minute),
	}
	result.appendEventLocked(time.Now(), "service", "service_started", "蓝牙解锁服务已启动", "BLE 扫描、会话监视和认证状态机已就绪", "运行中", false)
	return result
}

func (c *Coordinator) appendEventLocked(now time.Time, kind, code, message, detail, result string, warning bool) bool {
	if count := len(c.recentEvents); count > 0 {
		last := c.recentEvents[count-1]
		if last.Code == code && last.Message == message && last.Detail == detail && last.Warning == warning {
			return false
		}
	}
	c.nextEventID++
	c.recentEvents = append(c.recentEvents, RecentEvent{
		ID: c.nextEventID, At: now, Kind: kind, Code: code, Message: message,
		Detail: detail, Result: result, Warning: warning,
	})
	if len(c.recentEvents) > maxRecentEvents {
		c.recentEvents = append([]RecentEvent(nil), c.recentEvents[len(c.recentEvents)-maxRecentEvents:]...)
	}
	return true
}

func (c *Coordinator) appendEvent(now time.Time, kind, code, message, detail, result string, warning bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.appendEventLocked(now, kind, code, message, detail, result, warning)
}

func settingsFor(cfg config.Config) proximity.Settings {
	settings := proximity.DefaultSettings()
	settings.UnlockRSSI = cfg.Thresholds.UnlockRSSI
	settings.LockRSSI = cfg.Thresholds.LockRSSI
	settings.UnlockWindow = time.Duration(cfg.Thresholds.UnlockWindowMS) * time.Millisecond
	settings.LockWindow = time.Duration(cfg.Thresholds.LockWindowMS) * time.Millisecond
	settings.ProofTimeout = time.Duration(cfg.Thresholds.ProofTimeoutMS) * time.Millisecond
	settings.ManualHoldAway = time.Duration(cfg.Thresholds.ManualHoldAwayMS) * time.Millisecond
	settings.ImmediateUnlock = cfg.ImmediateUnlock
	settings.FailureCooldownOn = cfg.FailureCooldown
	settings.DopplerPrediction = cfg.DopplerPrediction
	settings.DopplerSensitivity = cfg.DopplerSensitivity
	if cfg.HighSensitivity {
		settings.HighSensitivity = true
		settings.UnlockRSSI = cfg.Thresholds.HighSensitivityRSSI
		settings.LockRSSI = cfg.Thresholds.HighSensitivityRSSI - 8
		settings.UnlockWindow = 1500 * time.Millisecond
		settings.LockWindow = 10 * time.Second
		settings.ProofTimeout = 10 * time.Second
		settings.ManualHoldAway = 2 * time.Second
		settings.RequiredNearSamples = 1
	}
	if cfg.DopplerPrediction {
		settings.ProofTimeout = 10 * time.Second
	}
	return settings
}

func challengeIntervalFor(cfg config.Config, locked bool) time.Duration {
	if cfg.HighSensitivity || cfg.DopplerPrediction {
		if locked {
			// The first new near advertisement should start authentication almost
			// immediately while retaining a small retry limit after failures.
			return 200 * time.Millisecond
		}
		return time.Second
	}
	return 3 * time.Second
}

func authenticationTimeoutFor(cfg config.Config) time.Duration {
	if cfg.HighSensitivity || cfg.DopplerPrediction {
		// Do not let a stale GATT connection block a newly returned phone for
		// the normal five-second exchange timeout.
		return 1500 * time.Millisecond
	}
	return 5 * time.Second
}

func (c *Coordinator) Start(ctx context.Context) error {
	return c.transport.Start(ctx, c.handleCandidate)
}

func (c *Coordinator) Close() error { return c.transport.Close() }

func (c *Coordinator) MarkSessionActive(sessionID uint32) {
	now := time.Now()
	c.mu.Lock()
	changed := !c.sessionActive || c.locked || c.activeSession != sessionID
	wasLocked := c.locked
	c.activeSession = sessionID
	c.sessionActive = true
	c.locked = false
	if changed {
		c.appendEventLocked(now, "session", "session_active", "本地控制台已进入桌面", fmt.Sprintf("会话 %d 已激活，锁定计时已重置", sessionID), "活动", false)
		pendingAge := now.Sub(c.predictionUnlockPendingAt)
		c.predictionGuardActive = wasLocked && c.predictionUnlockPending && pendingAge >= 0 && pendingAge <= 6*time.Second
		c.predictionUnlockPending = false
		c.predictionUnlockPendingAt = time.Time{}
		if c.predictionGuardActive {
			c.appendEventLocked(now, "authentication", "doppler_guard_started", "趋势预测解锁进入安全复核", "需要在 10 秒内再次完成手机签名认证，否则电脑将重新锁定", "复核中", false)
		}
	}
	c.mu.Unlock()
	c.state.OnUnlock(now)
	c.authorizer.Cancel()
}

func (c *Coordinator) MarkLocked(sessionID uint32, now time.Time) {
	c.mu.Lock()
	if c.sessionActive && sessionID == c.activeSession {
		changed := !c.locked
		c.locked = true
		c.predictionUnlockPending = false
		c.predictionUnlockPendingAt = time.Time{}
		c.predictionGuardActive = false
		if changed {
			c.appendEventLocked(now, "session", "session_locked", "Windows 已进入锁屏", "等待手机返回并完成新鲜挑战", "已锁定", false)
		}
		c.mu.Unlock()
		c.state.OnLock(now)
		return
	}
	c.mu.Unlock()
}

func (c *Coordinator) MarkUnlocked(sessionID uint32) { c.MarkSessionActive(sessionID) }

func (c *Coordinator) MarkLoggedOff(sessionID uint32) {
	now := time.Now()
	c.mu.Lock()
	if c.activeSession == sessionID {
		c.sessionActive = false
		c.locked = false
		c.activeSession = 0
		c.predictionUnlockPending = false
		c.predictionUnlockPendingAt = time.Time{}
		c.predictionGuardActive = false
		c.appendEventLocked(now, "session", "session_logged_off", "本地控制台已注销", "自动解锁授权已全部清除", "已注销", false)
	}
	c.mu.Unlock()
	c.authorizer.Cancel()
}

func (c *Coordinator) MarkResume(now time.Time) {
	c.mu.Lock()
	locked := c.locked && c.sessionActive
	c.mu.Unlock()
	if locked {
		c.state.OnResume(now)
		c.appendEvent(now, "session", "system_resumed", "系统已从睡眠恢复", "已打开一次新的近距离挑战窗口", "已恢复", false)
	}
}

func (c *Coordinator) Status(now time.Time) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	rssi, hasRSSI := c.state.MedianRSSI(now)
	prediction := c.state.Prediction(now)
	pausedUntil := time.Time{}
	if c.config.PausedUntil != nil {
		pausedUntil = *c.config.PausedUntil
	}
	lockPolicyActive := c.config.AutoLock || c.predictionGuardActive
	shouldLock := lockPolicyActive && c.sessionActive && !c.locked && !pausedUntil.After(now) && c.state.ShouldAutoLock(now)
	return Status{
		Configured:            c.config.TargetSID != "" && c.config.PCID != "",
		Paired:                c.config.PhoneID != "",
		CredentialValid:       c.config.CredentialValid,
		Mode:                  c.config.Mode,
		AutoLock:              c.config.AutoLock,
		HighSensitivity:       c.config.HighSensitivity,
		DopplerPrediction:     c.config.DopplerPrediction,
		DopplerSensitivity:    c.config.DopplerSensitivity,
		DopplerTriggered:      prediction.Ready,
		PredictedRSSI:         prediction.ProjectedRSSI,
		RSSISlopeDBPerSec:     prediction.SlopeDBPerSec,
		ImmediateUnlock:       c.config.ImmediateUnlock,
		FailureCooldown:       c.config.FailureCooldown,
		PausedUntil:           pausedUntil,
		SessionActive:         c.sessionActive,
		Locked:                c.locked,
		MedianRSSI:            rssi,
		HasRSSI:               hasRSSI,
		LastAuthenticated:     c.state.LastProof(),
		ShouldLock:            shouldLock,
		BLEBackend:            c.transport.Backend(),
		Authorization:         c.authorizer.Snapshot(now),
		CooldownUntil:         c.state.CooldownUntil(),
		LastAuthFailureCode:   c.lastAuthFailureCode,
		LastAuthFailureReason: c.lastAuthFailureReason,
		LastAuthFailureAt:     c.lastAuthFailureAt,
		LastCPEvent:           c.lastCPEvent,
		LastCPEventAt:         c.lastCPEventAt,
		UnlockRSSI:            c.config.Thresholds.UnlockRSSI,
		LockRSSI:              c.config.Thresholds.LockRSSI,
		HighSensitivityRSSI:   c.config.Thresholds.HighSensitivityRSSI,
		RecentEvents:          append([]RecentEvent(nil), c.recentEvents...),
	}
}

func (c *Coordinator) HandleControl(_ context.Context, request ipc.ControlRequest) ipc.ControlResponse {
	now := time.Now()
	switch request.Op {
	case "status":
		return ipc.ControlResponse{OK: true, Payload: c.Status(now)}
	case "session_active":
		var payload struct {
			SessionID uint32 `json:"session_id"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.SessionID == 0 {
			return ipc.ControlResponse{Error: "invalid session"}
		}
		c.mu.Lock()
		validator := c.sessionValid
		c.mu.Unlock()
		if validator != nil && !validator(payload.SessionID) {
			return ipc.ControlResponse{Error: "session is not the active local console"}
		}
		c.MarkSessionActive(payload.SessionID)
		return ipc.ControlResponse{OK: true}
	case "set_mode":
		var payload struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			return ipc.ControlResponse{Error: "invalid mode request"}
		}
		if _, err := protocol.ParseMode(payload.Mode); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.Mode = payload.Mode }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.appendEvent(now, "configuration", "mode_changed", "解锁模式已更新", "当前模式："+payload.Mode, "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_auto_lock":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Enabled == nil {
			return ipc.ControlResponse{Error: "invalid auto-lock request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.AutoLock = *payload.Enabled }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.appendEvent(now, "configuration", "auto_lock_changed", "自动锁定设置已更新", "自动锁定："+enabledText(*payload.Enabled), "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_high_sensitivity":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Enabled == nil {
			return ipc.ControlResponse{Error: "invalid high-sensitivity request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.HighSensitivity = *payload.Enabled }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		settings := settingsFor(c.config)
		c.mu.Unlock()
		c.state.UpdateSettings(settings)
		c.state.BeginProofGrace(now)
		c.appendEvent(now, "configuration", "high_sensitivity_changed", "高灵敏模式设置已更新", "高灵敏模式："+enabledText(*payload.Enabled), "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_doppler_prediction":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Enabled == nil {
			return ipc.ControlResponse{Error: "invalid doppler-prediction request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.DopplerPrediction = *payload.Enabled }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		settings := settingsFor(c.config)
		c.predictionActive = false
		c.predictionUnlockPending = false
		c.predictionUnlockPendingAt = time.Time{}
		c.predictionGuardActive = false
		c.mu.Unlock()
		c.state.UpdateSettings(settings)
		c.state.BeginProofGrace(now)
		c.appendEvent(now, "configuration", "doppler_prediction_changed", "多普勒预测设置已更新", "多普勒预测："+enabledText(*payload.Enabled)+"；预测只会提前发起认证，仍需有效手机签名", "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_doppler_sensitivity":
		var payload struct {
			Sensitivity int `json:"sensitivity"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			return ipc.ControlResponse{Error: "invalid doppler-sensitivity request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.DopplerSensitivity = payload.Sensitivity }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		settings := settingsFor(c.config)
		dopplerEnabled := c.config.DopplerPrediction
		c.predictionActive = false
		c.mu.Unlock()
		c.state.UpdateSettings(settings)
		if dopplerEnabled {
			c.state.BeginProofGrace(now)
		}
		c.appendEvent(now, "configuration", "doppler_sensitivity_changed", "多普勒预测灵敏度已更新", fmt.Sprintf("预测灵敏度：%d；数值越高越早发起认证", payload.Sensitivity), "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_high_sensitivity_threshold":
		var payload struct {
			RSSI int `json:"rssi"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			return ipc.ControlResponse{Error: "invalid high-sensitivity threshold request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.Thresholds.HighSensitivityRSSI = payload.RSSI }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		settings := settingsFor(c.config)
		highSensitivity := c.config.HighSensitivity
		c.mu.Unlock()
		c.state.UpdateSettings(settings)
		if highSensitivity {
			c.state.BeginProofGrace(now)
		}
		c.appendEvent(now, "configuration", "high_sensitivity_threshold_changed", "高灵敏触发阈值已更新", fmt.Sprintf("触发阈值 %d dBm；派生锁定线 %d dBm", payload.RSSI, payload.RSSI-8), "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_immediate_unlock":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Enabled == nil {
			return ipc.ControlResponse{Error: "invalid immediate-unlock request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.ImmediateUnlock = *payload.Enabled }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.state.SetImmediateUnlock(*payload.Enabled)
		c.appendEvent(now, "configuration", "immediate_unlock_changed", "锁屏后立即解锁设置已更新", "立即解锁："+enabledText(*payload.Enabled), "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_failure_cooldown":
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Enabled == nil {
			return ipc.ControlResponse{Error: "invalid failure-cooldown request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) { cfg.FailureCooldown = *payload.Enabled }); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		settings := settingsFor(c.config)
		c.mu.Unlock()
		c.state.UpdateSettings(settings)
		c.appendEvent(now, "configuration", "failure_cooldown_changed", "认证失败冷却设置已更新", "安全冷却："+enabledText(*payload.Enabled), "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "set_thresholds":
		var payload struct {
			UnlockRSSI          int  `json:"unlock_rssi"`
			LockRSSI            int  `json:"lock_rssi"`
			HighSensitivityRSSI *int `json:"high_sensitivity_rssi"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			return ipc.ControlResponse{Error: "invalid threshold request"}
		}
		if err := c.updateConfig(func(cfg *config.Config) {
			cfg.Thresholds.UnlockRSSI = payload.UnlockRSSI
			cfg.Thresholds.LockRSSI = payload.LockRSSI
			if payload.HighSensitivityRSSI != nil {
				cfg.Thresholds.HighSensitivityRSSI = *payload.HighSensitivityRSSI
			}
		}); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		settings := settingsFor(c.config)
		highSensitivity := c.config.HighSensitivity
		c.mu.Unlock()
		c.state.UpdateSettings(settings)
		if highSensitivity {
			c.state.BeginProofGrace(now)
		}
		detail := fmt.Sprintf("普通解锁 %d dBm；普通锁定 %d dBm", payload.UnlockRSSI, payload.LockRSSI)
		if payload.HighSensitivityRSSI != nil {
			detail += fmt.Sprintf("；高灵敏触发 %d dBm；派生锁定 %d dBm", *payload.HighSensitivityRSSI, *payload.HighSensitivityRSSI-8)
		}
		c.appendEvent(now, "configuration", "thresholds_changed", "距离阈值已更新", detail, "已保存", false)
		return ipc.ControlResponse{OK: true}
	case "pause":
		var payload struct {
			Seconds int `json:"seconds"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil || payload.Seconds < 0 || payload.Seconds > 86400 {
			return ipc.ControlResponse{Error: "invalid pause duration"}
		}
		until := now.Add(time.Duration(payload.Seconds) * time.Second)
		if payload.Seconds == 0 {
			until = time.Time{}
		}
		if err := c.updateConfig(func(cfg *config.Config) {
			if until.IsZero() {
				cfg.PausedUntil = nil
			} else {
				cfg.PausedUntil = &until
			}
		}); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		if until.IsZero() {
			c.appendEvent(now, "configuration", "service_resumed", "蓝牙解锁已恢复", "暂停状态已清除", "运行中", false)
		} else {
			c.appendEvent(now, "configuration", "service_paused", "蓝牙解锁已暂停", fmt.Sprintf("暂停 %d 秒，期间不会自动锁定或解锁", payload.Seconds), "已暂停", false)
		}
		return ipc.ControlResponse{OK: true, Payload: map[string]any{"paused_until": until}}
	case "pair_start":
		uri, err := c.startPairing(now)
		if err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.appendEvent(now, "pairing", "pairing_started", "已生成新的配对二维码", "二维码两分钟内有效，旧的一次性配对密钥已作废", "等待扫码", false)
		return ipc.ControlResponse{OK: true, Payload: map[string]any{"uri": uri, "expires_at": now.Add(2 * time.Minute)}}
	case "revoke":
		if err := c.revoke(); err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.appendEvent(now, "pairing", "pairing_revoked", "手机配对已撤销", "配对密钥、设备公钥和现有解锁授权已清除", "已撤销", false)
		return ipc.ControlResponse{OK: true}
	case "reload":
		next, err := config.Load(c.configPath)
		if err != nil {
			return ipc.ControlResponse{Error: err.Error()}
		}
		c.mu.Lock()
		previousHighSensitivity := c.config.HighSensitivity
		previousDopplerPrediction := c.config.DopplerPrediction
		if next.TargetSID != c.config.TargetSID || next.PCID != c.config.PCID {
			c.mu.Unlock()
			return ipc.ControlResponse{Error: "identity fields cannot be changed while the service is running"}
		}
		c.config = next
		c.predictionActive = false
		c.mu.Unlock()
		c.state.UpdateSettings(settingsFor(next))
		if previousHighSensitivity != next.HighSensitivity || previousDopplerPrediction != next.DopplerPrediction {
			c.state.BeginProofGrace(now)
		}
		if next.CredentialValid {
			c.authorizer.Enable()
		} else {
			c.authorizer.Disable()
		}
		return ipc.ControlResponse{OK: true}
	default:
		return ipc.ControlResponse{Error: "unknown operation"}
	}
}

func (c *Coordinator) HandleAuth(_ context.Context, command string) ipc.AuthResponse {
	now := time.Now()
	if strings.HasPrefix(command, "DIAG ") {
		event := strings.TrimSpace(strings.TrimPrefix(command, "DIAG "))
		c.mu.Lock()
		changed := c.lastCPEvent != event
		c.lastCPEvent = event
		c.lastCPEventAt = now
		if changed {
			c.appendEventLocked(now, "credential", "credential_provider_"+strings.ToLower(event), "凭据提供程序状态已更新", "事件："+event, "已接收", false)
		}
		c.mu.Unlock()
		return ipc.AuthResponse{Status: ipc.AuthUnavailable}
	}
	switch command {
	case "PEEK":
		if grant, ok := c.authorizer.Peek(now); ok {
			c.mu.Lock()
			validSession := c.sessionActive && c.locked && c.activeSession == grant.SessionID
			username := c.config.CanonicalUser
			targetSID := c.config.TargetSID
			c.mu.Unlock()
			if !validSession {
				c.authorizer.Cancel()
				return ipc.AuthResponse{Status: ipc.AuthUnavailable}
			}
			return ipc.AuthResponse{Status: ipc.AuthAvailable, Username: username, TargetSID: targetSID}
		}
		return ipc.AuthResponse{Status: ipc.AuthUnavailable}
	case "CONSUME":
		grant, err := c.authorizer.Consume(now)
		if err != nil {
			return ipc.AuthResponse{Status: ipc.AuthUnavailable}
		}
		c.mu.Lock()
		cfg := c.config
		validSession := c.sessionActive && c.locked && c.activeSession == grant.SessionID
		c.mu.Unlock()
		if !validSession || !cfg.CredentialValid {
			return ipc.AuthResponse{Status: ipc.AuthUnavailable}
		}
		password, err := c.secrets.Get(secret.CredentialName(cfg.TargetSID))
		if err != nil {
			return ipc.AuthResponse{Status: ipc.AuthError}
		}
		defer protocol.Zero(password)
		c.appendEvent(now, "credential", "credential_consumed", "自动解锁凭据已安全提交", "仅向当前锁定会话提交一次，授权消费后立即失效", "已提交", false)
		return ipc.AuthResponse{Status: ipc.AuthAvailable, Username: cfg.CanonicalUser, Password: string(password), TargetSID: cfg.TargetSID}
	case "INVALID":
		c.invalidateCredential()
		c.appendEvent(now, "credential", "credential_invalid", "Windows 拒绝了自动解锁凭据", "已清除一次性授权并禁用自动解锁，请手动登录后更新密码", "失败", true)
		return ipc.AuthResponse{Status: ipc.AuthInvalid}
	case "SUCCESS":
		c.authorizer.Cancel()
		c.appendEvent(now, "credential", "unlock_success", "Windows 已接受自动解锁凭据", "一次性授权已消费并清除", "成功", false)
		return ipc.AuthResponse{Status: ipc.AuthUnavailable}
	default:
		return ipc.AuthResponse{Status: ipc.AuthError}
	}
}

func (c *Coordinator) invalidateCredential() {
	c.authorizer.Disable()
	c.mu.Lock()
	c.config.CredentialValid = false
	next := c.config
	targetSID := c.config.TargetSID
	c.mu.Unlock()
	// Remove the rejected password first. Even if persisting config fails, a
	// service restart cannot retrieve and submit the stale credential again.
	_ = c.secrets.Delete(secret.CredentialName(targetSID))
	_ = config.Save(c.configPath, next)
}

func (c *Coordinator) handleCandidate(ctx context.Context, candidate ble.Candidate) {
	c.mu.Lock()
	cfg := c.config
	pairing := c.pairing
	if pairing != nil && time.Now().After(pairing.expiresAt) {
		protocol.Zero(pairing.secret[:])
		c.pairing = nil
		pairing = nil
	}
	c.mu.Unlock()
	pcID, err := decodeID(cfg.PCID)
	if err != nil {
		return
	}
	if pairing != nil && protocol.VerifyPairingAdvertisement(candidate.ServiceData, pairing.secret[:], pcID) {
		c.completePairing(ctx, candidate, pcID, pairing)
		return
	}
	if cfg.PhoneID == "" || cfg.PresenceSecretID == "" {
		return
	}
	presenceKey, err := c.secrets.Get(secret.PresenceName(cfg.PresenceSecretID))
	if err != nil {
		return
	}
	ad, err := protocol.ParseAdvertisement(candidate.ServiceData)
	valid := err == nil && ad.Verify(presenceKey)
	protocol.Zero(presenceKey)
	if !valid {
		return
	}
	now := time.Now()
	c.state.ObserveRSSI(candidate.RSSI, now)
	prediction := c.state.Prediction(now)
	c.mu.Lock()
	if cfg.DopplerPrediction {
		if prediction.Ready && !c.predictionActive {
			c.appendEventLocked(now, "authentication", "doppler_prediction_triggered", "多普勒预测已提前发起认证", fmt.Sprintf("当前 %d dBm；预测 %d dBm；增强速度 %.1f dB/s；仍需手机签名", candidate.RSSI, prediction.ProjectedRSSI, prediction.SlopeDBPerSec), "预测命中", false)
		}
		c.predictionActive = prediction.Ready
	} else {
		c.predictionActive = false
	}
	paused := cfg.PausedUntil != nil && cfg.PausedUntil.After(now)
	locked := c.locked
	shouldChallenge := c.sessionActive && !paused && !c.challengeBusy && c.state.CanAuthenticate(now) && now.Sub(c.lastChallenge) >= challengeIntervalFor(cfg, locked) && (!locked || c.state.ShouldAttemptUnlock(now))
	if shouldChallenge {
		c.challengeBusy = true
		c.lastChallenge = now
	}
	c.mu.Unlock()
	if !shouldChallenge {
		return
	}
	go func() {
		defer func() {
			c.mu.Lock()
			c.challengeBusy = false
			c.mu.Unlock()
		}()
		c.recordAuthenticationResult(c.authenticate(ctx, candidate))
	}()
}

func (c *Coordinator) recordAuthenticationResult(err error) {
	now := time.Now()
	if err == nil {
		c.mu.Lock()
		wasFailing := strings.HasPrefix(c.lastAuthOutcome, "failure:")
		shouldLog := c.lastAuthOutcome != "success"
		c.lastAuthOutcome = "success"
		logger := c.authLogger
		if shouldLog {
			code := "authentication_success"
			message := "手机认证通过"
			if wasFailing {
				code = "authentication_recovered"
				message = "手机认证已恢复"
			}
			c.appendEventLocked(now, "authentication", code, message, "电脑签名、手机签名、nonce、计数器和目标会话校验通过", "成功", false)
		}
		c.mu.Unlock()
		if shouldLog && logger != nil {
			code := "authentication_success"
			message := "手机认证通过"
			if wasFailing {
				code = "authentication_recovered"
				message = "手机认证已恢复"
			}
			logger(AuthenticationLogEntry{Code: code, Message: message})
		}
		return
	}
	failure := classifyAuthenticationError(err)
	c.state.RecordAttemptFailure(now)
	c.mu.Lock()
	c.lastAuthFailureCode = failure.code
	c.lastAuthFailureReason = failure.message
	c.lastAuthFailureAt = now
	logger := c.authLogger
	outcome := "failure:" + failure.code
	shouldLog := c.lastAuthOutcome != outcome
	c.lastAuthOutcome = outcome
	if shouldLog {
		detail := "错误代码：" + failure.code + "；将继续认证，并按普通模式超时策略决定是否锁定"
		if c.config.HighSensitivity || c.config.DopplerPrediction {
			detail = "错误代码：" + failure.code + "；将继续认证，连续失败满 10 秒后才允许自动锁定"
		}
		c.appendEventLocked(now, "authentication", failure.code, "认证失败："+failure.message, detail, "失败", true)
	}
	c.mu.Unlock()
	if failure.security {
		c.state.RecordFailure(now)
	}
	if shouldLog && logger != nil {
		logger(AuthenticationLogEntry{Warning: true, Code: failure.code, Message: failure.message})
	}
}

func (c *Coordinator) authenticate(parent context.Context, candidate ble.Candidate) error {
	c.mu.Lock()
	cfg := c.config
	sessionID := c.activeSession
	locked := c.locked
	c.mu.Unlock()
	predictionReady := cfg.DopplerPrediction && c.state.Prediction(time.Now()).Ready
	mode, err := protocol.ParseMode(cfg.Mode)
	if err != nil {
		return authError("config_invalid", "解锁模式配置无效", false, err)
	}
	pcID, err := decodeID(cfg.PCID)
	if err != nil {
		return authError("config_invalid", "电脑标识配置无效", false, err)
	}
	phoneID, err := decodeID(cfg.PhoneID)
	if err != nil {
		return authError("config_invalid", "手机标识配置无效", false, err)
	}
	challenge, err := protocol.NewChallenge(mode, pcID, phoneID, sessionID, cfg.TargetSID, time.Now())
	if err != nil {
		return authError("challenge_create_failed", "无法创建新鲜挑战", false, err)
	}
	challenge.Signature, err = c.signer.Sign(challenge.SigningBytes())
	if err != nil {
		return authError("pc_signing_failed", "电脑身份密钥签名失败", false, err)
	}
	wire, err := challenge.MarshalBinary()
	if err != nil {
		return authError("challenge_encode_failed", "无法编码蓝牙挑战", false, err)
	}
	ctx, cancel := context.WithTimeout(parent, authenticationTimeoutFor(cfg))
	defer cancel()
	responseWire, err := c.transport.Exchange(ctx, candidate, protocol.MessageChallenge, wire)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return authError("transport_timeout", "BLE 挑战响应超时", false, err)
		}
		return authError("transport_failure", "BLE GATT 挑战交换失败", false, err)
	}
	response, err := protocol.ParseResponse(responseWire)
	if err != nil || !response.Matches(challenge, time.Now()) {
		return authError("response_invalid", "手机响应格式或挑战绑定无效", true, err)
	}
	keyData := cfg.PhoneStrictKey
	if mode == protocol.ModeConvenience {
		keyData = cfg.PhoneRelaxedKey
	}
	publicBytes, err := base64.RawURLEncoding.DecodeString(keyData)
	if err != nil {
		return authError("config_invalid", "手机公钥配置无效", false, err)
	}
	publicKey, err := windowskey.ParsePublic(publicBytes)
	if err != nil || !protocol.VerifyP256(publicKey, response.SigningBytes(), response.Signature[:]) {
		return authError("signature_invalid", "手机签名验证失败", true, err)
	}
	if err := c.replay.Accept(response.Nonce, response.Counter, time.Now()); err != nil {
		return authError("replay_rejected", "手机响应计数器或 nonce 重放被拒绝", true, err)
	}
	proofAt := time.Now()
	c.state.RecordProof(proofAt)
	if locked {
		if err := c.authorizer.Grant(sessionID, proofAt); err != nil {
			return authError("authorization_failed", "无法创建一次性解锁授权", false, err)
		}
		c.mu.Lock()
		c.predictionUnlockPending = predictionReady
		if predictionReady {
			c.predictionUnlockPendingAt = proofAt
		} else {
			c.predictionUnlockPendingAt = time.Time{}
		}
		c.mu.Unlock()
		c.appendEvent(proofAt, "authorization", "authorization_granted", "一次性解锁授权已就绪", fmt.Sprintf("目标会话：%d；授权将在短时间内过期且只能消费一次", sessionID), "待消费", false)
	} else {
		c.mu.Lock()
		if c.predictionGuardActive {
			c.predictionGuardActive = false
			c.appendEventLocked(proofAt, "authentication", "doppler_guard_confirmed", "趋势预测解锁复核通过", "已在 10 秒安全窗口内再次完成手机签名认证", "已确认", false)
		}
		c.mu.Unlock()
	}
	return nil
}

func (c *Coordinator) startPairing(now time.Time) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pcID, err := decodeID(c.config.PCID)
	if err != nil {
		return "", err
	}
	publicKey, err := c.signer.PublicKey()
	if err != nil {
		return "", err
	}
	publicBytes, err := windowskey.MarshalPublic(publicKey)
	if err != nil {
		return "", err
	}
	secretBytes, err := protocol.RandomBytes(32)
	if err != nil {
		return "", err
	}
	pairing := &pendingPairing{expiresAt: now.Add(2 * time.Minute)}
	copy(pairing.secret[:], secretBytes)
	protocol.Zero(secretBytes)
	if c.pairing != nil {
		protocol.Zero(c.pairing.secret[:])
	}
	c.pairing = pairing
	uri := protocol.PairingURI{PCID: pcID, PCPublicKey: publicBytes, PairingSecret: pairing.secret, ExpiresAt: pairing.expiresAt}
	return uri.String(), nil
}

func (c *Coordinator) completePairing(ctx context.Context, candidate ble.Candidate, pcID [protocol.DeviceIDSize]byte, pending *pendingPairing) {
	challenge, err := protocol.NewPairingChallenge(pcID, pending.secret[:])
	if err != nil {
		return
	}
	exchangeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	wire, err := c.transport.Exchange(exchangeCtx, candidate, protocol.MessagePairing, challenge.MarshalBinary())
	if err != nil {
		return
	}
	response, err := protocol.ParsePairingResponse(wire, pending.secret[:], challenge)
	if err != nil {
		return
	}
	if time.Now().After(pending.expiresAt) {
		return
	}
	if _, err := windowskey.ParsePublic(response.StrictPublicKey[:]); err != nil {
		return
	}
	if _, err := windowskey.ParsePublic(response.RelaxedPublicKey[:]); err != nil {
		return
	}
	presenceKey, err := protocol.DerivePresenceKey(pending.secret[:], pcID, response.PhoneID)
	if err != nil {
		return
	}
	secretIDBytes, _ := protocol.RandomBytes(16)
	secretID := base64.RawURLEncoding.EncodeToString(secretIDBytes)
	protocol.Zero(secretIDBytes)
	if err := c.secrets.Put(secret.PresenceName(secretID), presenceKey); err != nil {
		protocol.Zero(presenceKey)
		return
	}
	protocol.Zero(presenceKey)
	err = c.updateConfig(func(cfg *config.Config) {
		cfg.PhoneID = base64.RawURLEncoding.EncodeToString(response.PhoneID[:])
		cfg.PhoneStrictKey = base64.RawURLEncoding.EncodeToString(response.StrictPublicKey[:])
		cfg.PhoneRelaxedKey = base64.RawURLEncoding.EncodeToString(response.RelaxedPublicKey[:])
		cfg.PresenceSecretID = secretID
	})
	if err != nil {
		_ = c.secrets.Delete(secret.PresenceName(secretID))
		return
	}
	c.state.ResetDeviceEvidence()
	c.replay.ResetCounter(0)
	c.mu.Lock()
	if c.pairing == pending {
		protocol.Zero(c.pairing.secret[:])
		c.pairing = nil
	}
	c.mu.Unlock()
	c.appendEvent(time.Now(), "pairing", "pairing_completed", "手机安全配对已完成", "已保存两种模式的手机公钥和新的 presence key；未记录完整设备标识", "成功", false)
}

func (c *Coordinator) revoke() error {
	c.mu.Lock()
	secretID := c.config.PresenceSecretID
	c.mu.Unlock()
	if secretID != "" {
		if err := c.secrets.Delete(secret.PresenceName(secretID)); err != nil {
			return err
		}
	}
	c.authorizer.Cancel()
	c.replay.ResetCounter(0)
	err := c.updateConfig(func(cfg *config.Config) {
		cfg.PhoneID = ""
		cfg.PhoneStrictKey = ""
		cfg.PhoneRelaxedKey = ""
		cfg.PresenceSecretID = ""
	})
	if err == nil {
		c.state.ResetDeviceEvidence()
	}
	return err
}

func (c *Coordinator) updateConfig(mutator func(*config.Config)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.config
	mutator(&next)
	if err := config.Save(c.configPath, next); err != nil {
		return err
	}
	c.config = next
	return nil
}

func decodeID(value string) ([protocol.DeviceIDSize]byte, error) {
	var result [protocol.DeviceIDSize]byte
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != len(result) {
		return result, fmt.Errorf("invalid device identifier")
	}
	copy(result[:], decoded)
	return result, nil
}

func enabledText(enabled bool) string {
	if enabled {
		return "已开启"
	}
	return "已关闭"
}
