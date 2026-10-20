package main

import (
	"sort"
	"time"

	"github.com/Anshuman115/heliolytics/internal/activity"
)

type Match struct {
	Prediction     int
	Reference      int
	IoU            float64
	OverlapSeconds int64
	UnionSeconds   int64
}

// Score is the one-to-one temporal matching for a single partition.
// A nil metric is undefined because its denominator is zero.
type Score struct {
	Predictions          int
	References           int
	TP                   int
	FP                   int
	FN                   int
	Precision            *float64
	Recall               *float64
	F1                   *float64
	Matches              []Match
	UnmatchedPredictions []int
	UnmatchedReferences  []int
}

// ScoreIntervals matches predictions to references with temporal IoU >= 0.5.
func ScoreIntervals(predictions, references []activity.Interval) Score {
	score := Score{Predictions: len(predictions), References: len(references)}
	score.Matches = maximumMatch(len(predictions), len(references), candidateEdges(predictions, references))

	matchedPrediction := make([]bool, len(predictions))
	matchedReference := make([]bool, len(references))
	for _, match := range score.Matches {
		matchedPrediction[match.Prediction] = true
		matchedReference[match.Reference] = true
	}
	for i := range predictions {
		if !matchedPrediction[i] {
			score.UnmatchedPredictions = append(score.UnmatchedPredictions, i)
		}
	}
	for i := range references {
		if !matchedReference[i] {
			score.UnmatchedReferences = append(score.UnmatchedReferences, i)
		}
	}

	score.TP = len(score.Matches)
	score.FP = score.Predictions - score.TP
	score.FN = score.References - score.TP
	if denominator := score.TP + score.FP; denominator > 0 {
		value := float64(score.TP) / float64(denominator)
		score.Precision = &value
	}
	if denominator := score.TP + score.FN; denominator > 0 {
		value := float64(score.TP) / float64(denominator)
		score.Recall = &value
	}
	if score.Precision != nil && score.Recall != nil {
		precision, recall := *score.Precision, *score.Recall
		if precision+recall > 0 {
			value := 2 * precision * recall / (precision + recall)
			score.F1 = &value
		}
	}
	return score
}

type edge struct {
	prediction int
	reference  int
	overlap    int64
	union      int64
	micro      int64
}

func candidateEdges(predictions, references []activity.Interval) []edge {
	edges := make([]edge, 0, len(predictions))
	for i, prediction := range predictions {
		for j, reference := range references {
			overlap := overlapSeconds(prediction, reference)
			if overlap <= 0 {
				continue
			}
			union := spanSeconds(prediction) + spanSeconds(reference) - overlap
			if union <= 0 || 2*overlap < union {
				continue
			}
			micro := (overlap*2*iouScale + union) / (2 * union)
			edges = append(edges, edge{prediction: i, reference: j, overlap: overlap, union: union, micro: micro})
		}
	}
	return edges
}

// Matching is max cardinality, then max total IoU. Cardinality weight dwarfs
// every reachable IoU sum, so the flow always prefers one more matched pair.
const (
	cardinalityWeight = int64(1) << 40
	iouScale          = int64(1_000_000)
)

func maximumMatch(predictionCount, referenceCount int, edges []edge) []Match {
	if len(edges) == 0 {
		return nil
	}
	predictionBase := 1
	referenceBase := predictionBase + predictionCount
	sink := referenceBase + referenceCount
	network := newFlowNetwork(sink + 1)
	for i := range predictionCount {
		network.addEdge(0, predictionBase+i, 1, 0)
	}
	for _, candidate := range edges {
		network.addEdge(predictionBase+candidate.prediction, referenceBase+candidate.reference, 1,
			candidate.micro-cardinalityWeight)
	}
	for j := range referenceCount {
		network.addEdge(referenceBase+j, sink, 1, 0)
	}
	for network.augment(0, sink) {
	}

	used := make(map[[2]int]edge, len(edges))
	for _, candidate := range edges {
		used[[2]int{candidate.prediction, candidate.reference}] = candidate
	}
	var matches []Match
	for i := range predictionCount {
		for _, flow := range network.graph[predictionBase+i] {
			if flow.capacity != 0 || flow.to < referenceBase || flow.to >= sink {
				continue
			}
			candidate, ok := used[[2]int{i, flow.to - referenceBase}]
			if !ok {
				continue
			}
			matches = append(matches, Match{
				Prediction:     candidate.prediction,
				Reference:      candidate.reference,
				IoU:            float64(candidate.overlap) / float64(candidate.union),
				OverlapSeconds: candidate.overlap,
				UnionSeconds:   candidate.union,
			})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Prediction != matches[j].Prediction {
			return matches[i].Prediction < matches[j].Prediction
		}
		return matches[i].Reference < matches[j].Reference
	})
	return matches
}

type flowEdge struct {
	to       int
	rev      int
	capacity int
	cost     int64
}

type flowNetwork struct {
	graph [][]flowEdge
}

func newFlowNetwork(nodes int) *flowNetwork {
	return &flowNetwork{graph: make([][]flowEdge, nodes)}
}

func (n *flowNetwork) addEdge(from, to, capacity int, cost int64) {
	n.graph[from] = append(n.graph[from], flowEdge{to: to, rev: len(n.graph[to]), capacity: capacity, cost: cost})
	n.graph[to] = append(n.graph[to], flowEdge{to: from, rev: len(n.graph[from]) - 1, capacity: 0, cost: -cost})
}

func (n *flowNetwork) augment(source, sink int) bool {
	const infinity = int64(1) << 60
	nodes := len(n.graph)
	distance := make([]int64, nodes)
	previousNode := make([]int, nodes)
	previousEdge := make([]int, nodes)
	queued := make([]bool, nodes)
	for i := range distance {
		distance[i] = infinity
		previousNode[i] = -1
	}
	distance[source] = 0
	queue := []int{source}
	queued[source] = true
	relaxations := 0
	limit := nodes * nodes
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		queued[node] = false
		for i := range n.graph[node] {
			step := &n.graph[node][i]
			if step.capacity == 0 {
				continue
			}
			candidate := distance[node] + step.cost
			if candidate >= distance[step.to] {
				continue
			}
			distance[step.to] = candidate
			previousNode[step.to] = node
			previousEdge[step.to] = i
			if queued[step.to] {
				continue
			}
			relaxations++
			if relaxations > limit {
				return false
			}
			queued[step.to] = true
			queue = append(queue, step.to)
		}
	}
	if previousNode[sink] < 0 {
		return false
	}
	path := make([]int, 0, nodes)
	for node := sink; node != source; {
		if previousNode[node] < 0 || len(path) > nodes {
			return false
		}
		path = append(path, node)
		node = previousNode[node]
	}
	for _, node := range path {
		from := previousNode[node]
		step := &n.graph[from][previousEdge[node]]
		step.capacity--
		n.graph[node][step.rev].capacity++
	}
	return true
}

func spanSeconds(interval activity.Interval) int64 {
	return int64(interval.End.Sub(interval.Start) / time.Second)
}

func overlapSeconds(a, b activity.Interval) int64 {
	start := a.Start
	if b.Start.After(start) {
		start = b.Start
	}
	end := a.End
	if b.End.Before(end) {
		end = b.End
	}
	if !end.After(start) {
		return 0
	}
	return int64(end.Sub(start) / time.Second)
}
