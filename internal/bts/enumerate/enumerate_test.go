package enumerate

import (
	"math/big"
	"testing"

	"github.com/reallyasi9/b1gpickem/internal/bts"
)

func Test_teamWeekMatrix_Add(t *testing.T) {
	remaining := bts.Remaining{
		"158",
		"164",
		"120",
		"84",
		"213",
		"127",
		"77",
		"130",
		"2509",
		"194",
		"2294",
		"275",
		"135",
		"356",
	}
	nWeeks := 14
	picksPerWeek := []int{0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2}
	cm := makeTeamWeekMatrix(remaining, nWeeks)
	type args struct {
		streak *bts.Streak
		weight *big.Int
	}
	tests := []struct {
		name string
		cm   *teamWeekMatrix
		args args
	}{
		{
			name: "2021 first valid streak",
			cm:   &cm,
			args: args{
				streak: bts.NewStreak(remaining, picksPerWeek),
				weight: big.NewInt(221),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cm.Add(tt.args.streak, tt.args.weight)
		})
	}
}
