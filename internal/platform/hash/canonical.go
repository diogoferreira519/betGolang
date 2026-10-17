package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type BusinessPayload struct {
	ProviderID string `json:"providerId"`
	ExternalTransactionID string `json:"externalTransactionId"`
	PlayerID string `json:"playerId"`
	WalletID string `json:"walletId"`
	RoundID string `json:"roundId"`
	GameID string `json:"gameId"`
	Kind string `json:"kind"`
	MoneyAmount string `json:"moneyAmount"`
	MoneyCurrency string `json:"moneyCurrency"`
	ReferenceExternalTransaction string `json:"referenceExternalTransactionId,omitempty"`
}

func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil { return nil, err }
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil { return nil, err }
	return json.Marshal(normalize(generic))
}

func PayloadHash(p BusinessPayload) (string, error) {
	b, err := CanonicalJSON(p)
	if err != nil { return "", err }
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func MessageBodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t { keys = append(keys, k) }
		sort.Strings(keys)
		out := make(map[string]any, len(t))
		for _, k := range keys { out[k] = normalize(t[k]) }
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t { out[i] = normalize(t[i]) }
		return out
	default:
		return t
	}
}
