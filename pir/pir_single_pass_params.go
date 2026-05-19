package pir

import (
	"math"
)

// SinglePassParams captures a runnable SinglePass configuration for a database.
type SinglePassParams struct {
	NumRows     int
	PaddedRows  int
	PaddingRows int
	RowLen      int
	SetSize     int
	NHints      int

	OfflineHintBytes     int
	ClientStateBytes     int
	OnlineBandwidthBytes int
}

// EstimateSinglePassParams derives a SinglePass configuration from the database
// size and row size. For setSize it picks a balanced default close to
// sqrt(numRows) and pads the database so that setSize divides the total rows.
func EstimateSinglePassParams(numRows, rowLen int) (SinglePassParams, error) {

	setSize := int(math.Ceil(math.Sqrt(float64(numRows))))

	paddedRows := ceilDiv(numRows, setSize) * setSize
	nHints := paddedRows / setSize

	permutationBytes := 0
	if nHints > 1 {
		permutationBytes = int(float64(paddedRows) * (math.Log2(float64(nHints)) / 8))
	}

	print("The total memory used by the db: ", rowLen*paddedRows)

	return SinglePassParams{
		NumRows:              numRows,
		PaddedRows:           paddedRows,
		PaddingRows:          paddedRows - numRows,
		RowLen:               rowLen,
		SetSize:              setSize,
		NHints:               nHints,
		OfflineHintBytes:     nHints * rowLen,
		ClientStateBytes:     (nHints * rowLen) + permutationBytes,
		OnlineBandwidthBytes: 2 * (setSize*rowLen + setSize*indexWidthBytes(nHints)),
	}, nil
}

func ceilDiv(x, y int) int {
	return (x + y - 1) / y
}

func indexWidthBytes(nHints int) int {
	if nHints <= 1 {
		return 0
	}
	return int(math.Log2(float64(nHints)) / 8)
}
