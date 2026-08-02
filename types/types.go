package types

import "hash/fnv"

type OBUData struct {
	EventID            string  `json:"event_id"`
	ProducedAtUnixNano int64   `json:"produced_at_unix_nano"`
	OBUID              int     `json:"obu_id"`
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	Payload            string  `json:"payload,omitempty"`
}

type Distance struct {
	EventID            string  `json:"event_id"`
	ProducedAtUnixNano int64   `json:"produced_at_unix_nano"`
	Value              float64 `json:"value"`
	OBUID              int     `json:"obu_id"`
}

type Invoice struct {
	OBUID     int     `json:"obu_id"`
	TotalDist float64 `json:"total_distance"`
	Amount    float64 `json:"amount"`
}

type EventReconciliation struct {
	Count uint64 `json:"event_count"`
	XOR   uint64 `json:"event_id_hash_xor"`
	Sum   uint64 `json:"event_id_hash_sum"`
}

func EventIDFingerprint(eventID string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(eventID))
	return hash.Sum64()
}
