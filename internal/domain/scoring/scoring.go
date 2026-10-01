// Package scoring holds the pure scoring rules for an attempt (BR-30..BR-33).
package scoring

import (
	"math"

	"github.com/di-eqa/backend/internal/domain/entity"
)

// Result is the outcome of scoring one attempt.
type Result struct {
	Correct    int
	Total      int
	Unanswered int
	Percent    float64
	Passed     bool
	PerType    []entity.TypeScore
}

// Score grades answers against the drawn questions.
//
//   - total is the number of drawn questions, never the number answered (BR-30);
//   - an unanswered or unknown answer counts as wrong (BR-30);
//   - passed uses integer arithmetic: correct*100 >= passPercent*total (BR-32);
//   - perType lists only cell types present in the drawn set, ordered like
//     cellTypes (BR-33).
func Score(questions []entity.AttemptQuestion, answers map[string]string, cellTypes []entity.CellTypeOption, passPercent int) Result {
	total := len(questions)
	res := Result{Total: total}

	type acc struct{ total, correct int }
	byType := map[string]*acc{}
	var seenOrder []string

	for _, q := range questions {
		a := byType[q.CorrectType]
		if a == nil {
			a = &acc{}
			byType[q.CorrectType] = a
			seenOrder = append(seenOrder, q.CorrectType)
		}
		a.total++

		given, answered := answers[q.ID]
		if !answered || given == "" {
			res.Unanswered++
			continue
		}
		if given == q.CorrectType {
			res.Correct++
			a.correct++
		}
	}

	if total > 0 {
		res.Percent = round2(float64(res.Correct) * 100 / float64(total))
		res.Passed = res.Correct*100 >= passPercent*total
	}

	labels := make(map[string]string, len(cellTypes))
	var order []string
	for _, t := range cellTypes {
		labels[t.Key] = t.Label
		order = append(order, t.Key)
	}
	// Types present in the draw but absent from the snapshot (should not
	// happen) are appended so no question is silently dropped from the breakdown.
	inOrder := make(map[string]bool, len(order))
	for _, k := range order {
		inOrder[k] = true
	}
	for _, k := range seenOrder {
		if !inOrder[k] {
			order = append(order, k)
			labels[k] = k
		}
	}

	res.PerType = []entity.TypeScore{}
	for _, k := range order {
		a := byType[k]
		if a == nil || a.total == 0 {
			continue
		}
		res.PerType = append(res.PerType, entity.TypeScore{
			Key:     k,
			Label:   labels[k],
			Total:   a.total,
			Correct: a.correct,
			Percent: round2(float64(a.correct) * 100 / float64(a.total)),
		})
	}
	return res
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
