package queue

type PublisherSnapshot struct {
	Depth         int
	Capacity      int
	UsageRatio    float64
	FailureRatio  float64
	RecentSamples int
}

type SnapshotProvider interface {
	Snapshot() PublisherSnapshot
}
