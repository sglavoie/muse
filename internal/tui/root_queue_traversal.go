package tui

import "github.com/sglavoie/muse/internal/tui/queue"

// queueTraversalSource adapts a player that exposes the playback timeline to
// the Queue view's read-only provider. A player without a timeline yields nil,
// which is how the view knows to keep rendering persisted queue order.
//
// The adapter exists so internal/tui/queue stays free of the player package,
// matching the rest of the view subpackages: it names the shape it needs and
// root supplies it.
func queueTraversalSource(p playPauser) queue.TraversalSource {
	r, ok := p.(queueTraversalReader)
	if !ok {
		return nil
	}
	return playerTraversalSource{reader: r}
}

// playerTraversalSource converts one player snapshot into one view snapshot.
// Both are already immutable copies, so nothing here needs to defend against
// the Manager mutating the order underneath the view.
type playerTraversalSource struct {
	reader queueTraversalReader
}

func (p playerTraversalSource) QueueTraversal() queue.Traversal {
	t := p.reader.QueueTraversal()
	return queue.Traversal{
		Order:           t.Order,
		CurrentPosition: t.CurrentPosition,
		Shuffled:        t.Shuffled,
	}
}
