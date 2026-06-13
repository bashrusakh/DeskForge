package utils

import (
	"errors"
	"sync"
	"time"
)

type SecurityPolicy struct {
	CaptchaThreshold int // ，0, 0
	BanThreshold     int // ，0
	AttemptsWindow   time.Duration
	BanDuration      time.Duration
}

type CaptchaProvider interface {
	Generate() (id string, content string, answer string, err error)
	//Validate(ip, code string) bool
	Expiration() time.Duration           // ,  AttemptsWindow
	Draw(content string) (string, error) // 
}

type CaptchaMeta struct {
	Id        string
	Content   string
	Answer    string
	ExpiresAt time.Time
}

// IP
type BanRecord struct {
	ExpiresAt time.Time
	Reason    string
}

type LoginLimiter struct {
	mu          sync.Mutex
	policy      SecurityPolicy
	attempts    map[string][]time.Time //
	captchas    map[string]CaptchaMeta
	bannedIPs   map[string]BanRecord
	provider    CaptchaProvider
	cleanupStop chan struct{}
}

var defaultSecurityPolicy = SecurityPolicy{
	CaptchaThreshold: 3,
	BanThreshold:     5,
	AttemptsWindow:   5 * time.Minute,
	BanDuration:      30 * time.Minute,
}

func NewLoginLimiter(policy SecurityPolicy) *LoginLimiter {

	if policy.AttemptsWindow == 0 {
		policy.AttemptsWindow = 5 * time.Minute
	}
	if policy.BanDuration == 0 {
		policy.BanDuration = 30 * time.Minute
	}

	ll := &LoginLimiter{
		policy:      policy,
		attempts:    make(map[string][]time.Time),
		captchas:    make(map[string]CaptchaMeta),
		bannedIPs:   make(map[string]BanRecord),
		cleanupStop: make(chan struct{}),
	}
	go ll.cleanupRoutine()
	return ll
}

func (ll *LoginLimiter) RegisterProvider(p CaptchaProvider) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	ll.provider = p
}

// isDisabled 
func (ll *LoginLimiter) isDisabled() bool {
	return ll.policy.CaptchaThreshold < 0 && ll.policy.BanThreshold == 0
}

func (ll *LoginLimiter) RecordFailedAttempt(ip string) {
	if ll.isDisabled() {
		return
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if banned, _ := ll.isBanned(ip); banned {
		return
	}

	now := time.Now()
	windowStart := now.Add(-ll.policy.AttemptsWindow)

	validAttempts := ll.pruneAttempts(ip, windowStart)

	validAttempts = append(validAttempts, now)
	ll.attempts[ip] = validAttempts

	if ll.policy.BanThreshold > 0 && len(validAttempts) >= ll.policy.BanThreshold {
		ll.banIP(ip, "excessive failed attempts")
		return
	}

	return
}

func (ll *LoginLimiter) RequireCaptcha() (error, CaptchaMeta) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if ll.provider == nil {
		return errors.New("no captcha provider available"), CaptchaMeta{}
	}

	id, content, answer, err := ll.provider.Generate()
	if err != nil {
		return err, CaptchaMeta{}
	}

	ll.captchas[id] = CaptchaMeta{
		Id:        id,
		Content:   content,
		Answer:    answer,
		ExpiresAt: time.Now().Add(ll.provider.Expiration()),
	}

	return nil, ll.captchas[id]
}

func (ll *LoginLimiter) VerifyCaptcha(id, answer string) bool {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if ll.provider == nil {
		return false
	}

	captcha, exists := ll.captchas[id]
	if !exists {
		return false
	}

	if time.Now().After(captcha.ExpiresAt) {
		delete(ll.captchas, id)
		return false
	}

	if answer == captcha.Answer {
		delete(ll.captchas, id)
		return true
	}

	return false
}

func (ll *LoginLimiter) DrawCaptcha(content string) (err error, str string) {
	str, err = ll.provider.Draw(content)
	return
}

func (ll *LoginLimiter) RemoveAttempts(ip string) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	_, exists := ll.attempts[ip]
	if exists {
		delete(ll.attempts, ip)
	}
}

// CheckSecurityStatus 
func (ll *LoginLimiter) CheckSecurityStatus(ip string) (banned bool, captchaRequired bool) {
	if ll.isDisabled() {
		return
	}
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if banned, _ = ll.isBanned(ip); banned {
		return
	}

	ll.pruneAttempts(ip, time.Now().Add(-ll.policy.AttemptsWindow))

	captchaRequired = len(ll.attempts[ip]) >= ll.policy.CaptchaThreshold

	return
}

func (ll *LoginLimiter) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ll.cleanupExpired()
		case <-ll.cleanupStop:
			return
		}
	}
}

func (ll *LoginLimiter) isBanned(ip string) (bool, BanRecord) {
	record, exists := ll.bannedIPs[ip]
	if !exists {
		return false, BanRecord{}
	}
	if time.Now().After(record.ExpiresAt) {
		delete(ll.bannedIPs, ip)
		return false, BanRecord{}
	}
	return true, record
}

func (ll *LoginLimiter) banIP(ip, reason string) {
	ll.bannedIPs[ip] = BanRecord{
		ExpiresAt: time.Now().Add(ll.policy.BanDuration),
		Reason:    reason,
	}
	delete(ll.attempts, ip)
	delete(ll.captchas, ip)
}

func (ll *LoginLimiter) pruneAttempts(ip string, cutoff time.Time) []time.Time {
	var valid []time.Time
	for _, t := range ll.attempts[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		delete(ll.attempts, ip)
	} else {
		ll.attempts[ip] = valid
	}
	return valid
}

func (ll *LoginLimiter) pruneCaptchas(id string) {
	if captcha, exists := ll.captchas[id]; exists {
		if time.Now().After(captcha.ExpiresAt) {
			delete(ll.captchas, id)
		}
	}
}

func (ll *LoginLimiter) cleanupExpired() {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	now := time.Now()

	for ip, record := range ll.bannedIPs {
		if now.After(record.ExpiresAt) {
			delete(ll.bannedIPs, ip)
		}
	}

	for ip := range ll.attempts {
		ll.pruneAttempts(ip, now.Add(-ll.policy.AttemptsWindow))
	}

	for id := range ll.captchas {
		ll.pruneCaptchas(id)
	}
}
