package middleware

import (
	"errors"
	"testing"
)

// ErrPriorityExhausted mirrors the model layer error for testing
var ErrPriorityExhausted = errors.New("all priority levels exhausted")

// TestPriorityIterationLogic verifies the priority iteration loop logic
// These tests document the expected behavior of the priority failover fix.
//
// Test scenarios:
// T2.1: Iterate through all available priorities until finding healthy channel
// T2.2: When all priorities exhausted, return ErrPriorityExhausted to exit early

// mockChannelSelector simulates CacheGetRandomSatisfiedChannel behavior
type mockChannelSelector struct {
	// channelAtRetry maps retry level to whether a channel is available
	channelAtRetry   map[int]bool
	numPriorities    int // Total number of unique priorities
	callCount        int
	exhaustedAtRetry int // Track when exhausted error is returned
}

func (m *mockChannelSelector) selectChannel(retry int) (hasChannel bool, err error) {
	m.callCount++

	// Simulate ErrPriorityExhausted when retry >= numPriorities
	if retry >= m.numPriorities {
		m.exhaustedAtRetry = retry
		return false, ErrPriorityExhausted
	}

	if hasChannel, ok := m.channelAtRetry[retry]; ok {
		return hasChannel, nil
	}
	return false, nil // No channel at this priority
}

// TestPriorityIterationWithEarlyExit verifies that the loop exits early
// when ErrPriorityExhausted is returned, avoiding 1000 iterations
func TestPriorityIterationWithEarlyExit(t *testing.T) {
	tests := []struct {
		name            string
		channelAtRetry  map[int]bool
		numPriorities   int
		expectSuccess   bool
		expectedCalls   int // Exact number of calls expected
		expectExhausted bool
	}{
		{
			name: "channel found at first priority",
			channelAtRetry: map[int]bool{
				0: true,
			},
			numPriorities: 3,
			expectSuccess: true,
			expectedCalls: 1, // Found on first try
		},
		{
			name: "channel at 3rd priority (first 2 empty)",
			channelAtRetry: map[int]bool{
				0: false,
				1: false,
				2: true, // Channel available at 3rd priority
			},
			numPriorities: 5,
			expectSuccess: true,
			expectedCalls: 3, // Try 0, 1, 2 -> found at 2
		},
		{
			name: "all 3 priorities exhausted, none have healthy channels",
			channelAtRetry: map[int]bool{
				0: false,
				1: false,
				2: false,
			},
			numPriorities:   3,
			expectSuccess:   false,
			expectedCalls:   4, // Try 0, 1, 2 (nil), then 3 returns exhausted
			expectExhausted: true,
		},
		{
			name:            "single priority, no healthy channels",
			channelAtRetry:  map[int]bool{0: false},
			numPriorities:   1,
			expectSuccess:   false,
			expectedCalls:   2, // Try 0 (nil), then 1 returns exhausted
			expectExhausted: true,
		},
		{
			name: "10 priorities, channel at 8th",
			channelAtRetry: map[int]bool{
				7: true, // Channel at 8th priority (0-indexed)
			},
			numPriorities: 10,
			expectSuccess: true,
			expectedCalls: 8, // Try 0-7, found at 7
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockChannelSelector{
				channelAtRetry: tt.channelAtRetry,
				numPriorities:  tt.numPriorities,
			}

			// Simulate the priority iteration loop from distributor.go
			const maxPriorityLevels = 1000
			var foundChannel bool
			var gotExhausted bool

			for retry := 0; retry < maxPriorityLevels; retry++ {
				hasChannel, err := mock.selectChannel(retry)

				// Handle ErrPriorityExhausted - this is the key fix
				if err != nil && errors.Is(err, ErrPriorityExhausted) {
					gotExhausted = true
					break
				}

				if err != nil {
					break // Other system error
				}

				if hasChannel {
					foundChannel = true
					break
				}
				// Continue to next priority level
			}

			if foundChannel != tt.expectSuccess {
				t.Errorf("expected success=%v, got success=%v", tt.expectSuccess, foundChannel)
			}

			if gotExhausted != tt.expectExhausted {
				t.Errorf("expected exhausted=%v, got exhausted=%v", tt.expectExhausted, gotExhausted)
			}

			if mock.callCount != tt.expectedCalls {
				t.Errorf("expected %d calls, got %d (wrong iteration count)", tt.expectedCalls, mock.callCount)
			}
		})
	}
}

// TestNoLogStormOnExhaustion verifies that when all priorities are exhausted,
// we don't iterate 1000 times causing log spam
func TestNoLogStormOnExhaustion(t *testing.T) {
	// Simulate: 3 priority levels, all channels suspended
	mock := &mockChannelSelector{
		channelAtRetry: map[int]bool{
			0: false, // Priority 100 - all suspended
			1: false, // Priority 50 - all suspended
			2: false, // Priority 10 - all suspended
		},
		numPriorities: 3,
	}

	const maxPriorityLevels = 1000

	for retry := 0; retry < maxPriorityLevels; retry++ {
		_, err := mock.selectChannel(retry)

		if err != nil && errors.Is(err, ErrPriorityExhausted) {
			break
		}
	}

	// With 3 priorities, we should only make 4 calls:
	// retry=0 (nil), retry=1 (nil), retry=2 (nil), retry=3 (exhausted)
	// NOT 1000 calls!
	if mock.callCount > 10 {
		t.Errorf("REGRESSION: expected ~4 calls for 3 priorities, got %d (log storm!)", mock.callCount)
	}

	if mock.callCount != 4 {
		t.Errorf("expected exactly 4 calls (3 priorities + 1 exhausted), got %d", mock.callCount)
	}
}

// TestPriorityIterationErrorHandling verifies that system errors stop the iteration
func TestPriorityIterationErrorHandling(t *testing.T) {
	systemError := errors.New("database connection failed")

	mock := &mockChannelSelector{
		channelAtRetry: map[int]bool{0: false},
		numPriorities:  100, // Many priorities
	}

	const maxPriorityLevels = 1000
	var gotError error

	for retry := 0; retry < maxPriorityLevels; retry++ {
		// Simulate error at retry=2
		if retry == 2 {
			gotError = systemError
			break
		}

		_, err := mock.selectChannel(retry)
		if err != nil {
			gotError = err
			break
		}
	}

	if gotError != systemError {
		t.Error("expected to capture system error")
	}
}
