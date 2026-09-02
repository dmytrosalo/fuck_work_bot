package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	errBadSignature = errors.New("invalid initData signature")
	errStaleAuth    = errors.New("initData is too old")
)

// verifyInitData validates a Telegram Mini App initData payload and returns
// the authenticated Telegram user id. Per Telegram's spec:
//
//	secret_key = HMAC_SHA256(key="WebAppData", data=bot_token)
//	hash       = HMAC_SHA256(key=secret_key,  data=data_check_string)
//
// where data_check_string is "key=value" pairs sorted by key, joined by "\n",
// excluding the hash field itself.
func verifyInitData(initData, botToken string, maxAge time.Duration) (int64, string, string, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, "", "", err
	}
	given := values.Get("hash")
	if given == "" {
		return 0, "", "", errBadSignature
	}
	values.Del("hash")

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(parts, "\n")))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(given)) {
		return 0, "", "", errBadSignature
	}

	ts, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return 0, "", "", errStaleAuth
	}
	if time.Since(time.Unix(ts, 0)) > maxAge {
		return 0, "", "", errStaleAuth
	}

	var user struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		Username  string `json:"username"`
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil {
		return 0, "", "", err
	}
	return user.ID, user.FirstName, user.Username, nil
}
