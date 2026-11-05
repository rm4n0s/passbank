package kit

import (
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/widgets"
)

// activateOnEnterOrSpace is the key handler of a focusable control: Enter and
// Space call act (when there is one).
func activateOnEnterOrSpace(act func()) widgets.FocusOnKeyEventCallback {
	return func(_ *widgets.FocusNode, ev services.KeyEvent) widgets.KeyEventResult {
		if _, down := ev.(*services.KeyDownEvent); !down || act == nil {
			return widgets.KeyEventResultIgnored
		}
		if k := ev.LogicalKey(); k == services.LogicalKeyboardKeyEnter || k == services.LogicalKeyboardKeySpace {
			act()
			return widgets.KeyEventResultHandled
		}
		return widgets.KeyEventResultIgnored
	}
}
