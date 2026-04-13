package types

type OBUData struct {
	OBUID int     `json:"obuId"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

type Distance struct {
	Value float64 `json:"value"`
	OBUID int     `json:"obuid"`
	Unix  int64   `json:"unix"`
}
