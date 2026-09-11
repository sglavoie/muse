package autodj

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/radio"
)

// This file holds the Recipes tab: the page where a settings snapshot is given
// a name, read back later, renamed, and eventually deleted.
//
// Two rules shape everything below. Saving writes settings and nothing else —
// no seed, no session, no queue row — so a save can never change what is
// playing. Loading writes the *draft* and nothing else, so reading a recipe
// back is an edit the user still has to Apply. Neither gesture touches the
// running session, which is what makes "no implicit playback changes from Save
// or Load" true by construction rather than by care.
//
// The third rule is about names. A name that already belongs to another recipe
// is refused and said so — here before the round trip, and again in storage,
// which is the authority. Silently overwriting the settings behind a name the
// user already gave to something else is the one outcome that loses their work
// without leaving a trace.

// ErrRecipeNameTaken reports a name that already belongs to another recipe. The
// panel raises it for a collision it can see in the list it holds, and the root
// model translates storage's own refusal onto it, so the user reads the same
// sentence whichever side caught it.
var ErrRecipeNameTaken = errors.New("a recipe with that name already exists")

// errRecipeNameRequired reports an empty name. A recipe is addressed by name in
// every gesture the tab offers, so an unnamed one would be unreachable.
var errRecipeNameRequired = errors.New("a recipe needs a name")

// errRecipeUnreadable reports a gesture asked of a recipe whose stored row this
// build could not read. Its settings are the zero value rather than the ones the
// user saved, so loading it would quietly replace their filters with none, and
// renaming it would write that emptiness back over the row.
var errRecipeUnreadable = errors.New("this recipe could not be read — it can only be deleted")

// errNoRecipe reports a gesture made with no recipe under the cursor.
var errNoRecipe = errors.New("no recipe selected")

// Recipe is one stored settings snapshot as the tab lists it.
type Recipe struct {
	// ID is the storage identity the root model addresses the recipe by.
	ID int64
	// Name is the user's label for it.
	Name string
	// Settings is the stored snapshot.
	Settings radio.Settings
	// Invalid records a stored row this build could not read, in which case
	// Settings is the zero value. Such a row is still listed — hiding it would
	// leave the user unable to clear it — but it may only be deleted.
	Invalid error
}

// RecipeOp names one durable recipe action the panel asks the root to perform.
//
// Loading is deliberately absent. It edits the draft and touches no storage, so
// the panel does it itself and the root never hears about it.
type RecipeOp int

const (
	// recipeNone is the zero request: nothing is pending.
	recipeNone RecipeOp = iota
	// RecipeSave stores the draft under a new name.
	RecipeSave
	// RecipeUpdate overwrites an existing recipe with the draft.
	RecipeUpdate
	// RecipeRename changes a recipe's name, keeping its stored settings.
	RecipeRename
	// RecipeDelete removes a recipe.
	RecipeDelete
)

// verb is the past-tense word the outcome notice is written with.
func (op RecipeOp) verb() string {
	switch op {
	case RecipeSave:
		return "Saved"
	case RecipeUpdate:
		return "Updated"
	case RecipeRename:
		return "Renamed"
	case RecipeDelete:
		return "Deleted"
	}
	return "Changed"
}

// RecipeRequest is one recipe action handed to the root model.
//
// Settings is carried even for a rename because storage rewrites a recipe's
// whole row: a rename must therefore hand back the settings the recipe already
// had, not the draft the user happens to be editing, or renaming a recipe would
// silently re-save it.
type RecipeRequest struct {
	// Op is what to do. recipeNone means nothing is pending.
	Op RecipeOp
	// ID is the recipe acted on, zero when saving a new one.
	ID int64
	// Name is the name to store.
	Name string
	// Settings is the snapshot to store.
	Settings radio.Settings
}

// RecipeOutcome is the root model's answer to one RecipeRequest.
type RecipeOutcome struct {
	// Op and Name echo the request, so the notice names what was acted on even
	// after the list underneath has been rebuilt.
	Op   RecipeOp
	Name string
	// Recipes is the list as storage now holds it. A nil list leaves the one on
	// screen alone, which is what a failed re-read should do.
	Recipes []Recipe
	// Err is the reason the action was refused, or nil.
	Err error
}

// namePrompt is the modal text entry that names a recipe.
//
// It carries the settings the request will store, captured when the prompt
// opened, so what is written is decided by the gesture rather than by whatever
// the draft has become by the time the user finishes typing.
type namePrompt struct {
	op       RecipeOp
	id       int64
	value    string
	settings radio.Settings
}

// title is the prompt's heading.
func (n namePrompt) title() string {
	if n.op == RecipeRename {
		return " Rename recipe "
	}
	return " Save recipe as "
}

// RecipesActive reports whether the Recipes page is showing. Like
// PreviewActive, it is the one thing about the panel's paging the root model
// needs to know.
func (p TunePanel) RecipesActive() bool { return p.tab == tabRecipes }

// PendingRecipe returns the request the last ActionRecipe asked for.
func (p TunePanel) PendingRecipe() RecipeRequest { return p.request }

// AckRecipe files the root model's answer to one recipe request.
//
// An answer that matches no pending request is ignored the way a late commit
// acknowledgement is: it belongs to a gesture made before this panel was opened,
// and filing it would put a note about work the user cannot see onto the page.
//
// A refusal reopens the prompt that produced it, with the rejected name still in
// the box. That is the whole point of refusing a collision rather than
// overwriting: the user is told, and is left holding exactly what they need to
// fix it.
func (p TunePanel) AckRecipe(res RecipeOutcome) TunePanel {
	req := p.request
	if req.Op == recipeNone {
		return p
	}
	p.request = RecipeRequest{}
	if res.Recipes != nil {
		p.recipes = append([]Recipe(nil), res.Recipes...)
	}
	p.recipeCursor = clampIndex(p.recipeCursor, len(p.recipes))
	if res.Err != nil {
		p.recipeNote = StatusLine{Text: res.Err.Error(), Tone: ToneError}
		if prompt, ok := retryPrompt(req, res); ok {
			p.prompt = &prompt
		}
		return p
	}
	p.recipeNote = StatusLine{Text: res.Op.verb() + " " + quoted(res.Name) + ".", Tone: ToneInfo}
	// The cursor follows the recipe that was just written. Storage lists recipes
	// by name, so a save or a rename reorders the page under the cursor — leaving
	// it on its old index would put it on a different recipe than the one the
	// user just named, and the next d would delete that one.
	if idx, ok := p.indexOfName(res.Name); ok && res.Op != RecipeDelete {
		p.recipeCursor = idx
	}
	return p
}

// indexOfName locates a recipe by the name storage now holds it under.
func (p TunePanel) indexOfName(name string) (int, bool) {
	for i, r := range p.recipes {
		if strings.EqualFold(r.Name, name) {
			return i, true
		}
	}
	return 0, false
}

// retryPrompt rebuilds the name prompt a refused request came from, so the user
// can correct the name instead of retyping it. A delete has no prompt to
// reopen, and an answer that does not match the request in flight belongs to a
// gesture this panel never made.
func retryPrompt(req RecipeRequest, res RecipeOutcome) (namePrompt, bool) {
	if req.Op != res.Op || (req.Op != RecipeSave && req.Op != RecipeRename) {
		return namePrompt{}, false
	}
	return namePrompt{op: req.Op, id: req.ID, value: req.Name, settings: req.Settings}, true
}

// focusedRecipe returns the recipe under the cursor.
func (p TunePanel) focusedRecipe() (Recipe, bool) {
	if p.recipeCursor < 0 || p.recipeCursor >= len(p.recipes) {
		return Recipe{}, false
	}
	return p.recipes[p.recipeCursor], true
}

// moveRecipeCursor wraps the cursor around the list.
func (p TunePanel) moveRecipeCursor(delta int) TunePanel {
	if len(p.recipes) == 0 {
		p.recipeCursor = 0
		return p
	}
	p.recipeCursor = (p.recipeCursor + len(p.recipes) + delta) % len(p.recipes)
	return p
}

// loadFocusedRecipe copies the focused recipe's settings into the draft.
//
// Nothing else moves. The applied snapshot, the running session, and the queue
// are all untouched, so a load is an edit like any other and the user still has
// to commit it. Tag references are copied verbatim, including ones the catalog
// can no longer resolve: dropping them here would silently widen what the DJ may
// play, and the Tune tab already names them and offers the repair.
func (p TunePanel) loadFocusedRecipe() TunePanel {
	r, ok := p.focusedRecipe()
	if !ok {
		return p.noteRecipeErr(errNoRecipe)
	}
	if r.Invalid != nil {
		return p.noteRecipeErr(errRecipeUnreadable)
	}
	p.draft = cloneSettings(r.Settings)
	p.err = nil
	p.recipeNote = StatusLine{
		Text: fmt.Sprintf("Loaded %s into the draft — ctrl+s to %s, esc to discard.",
			quoted(r.Name), strings.ToLower(p.CommitLabel())),
		Tone: ToneInfo,
	}
	return p
}

// openSavePrompt asks for a name for the draft.
func (p TunePanel) openSavePrompt() TunePanel {
	prompt := namePrompt{op: RecipeSave, settings: cloneSettings(p.draft)}
	p.prompt = &prompt
	p.recipeNote = StatusLine{}
	return p
}

// openRenamePrompt asks for a new name for the focused recipe.
func (p TunePanel) openRenamePrompt() TunePanel {
	r, ok := p.focusedRecipe()
	if !ok {
		return p.noteRecipeErr(errNoRecipe)
	}
	if r.Invalid != nil {
		return p.noteRecipeErr(errRecipeUnreadable)
	}
	prompt := namePrompt{op: RecipeRename, id: r.ID, value: r.Name, settings: cloneSettings(r.Settings)}
	p.prompt = &prompt
	p.recipeNote = StatusLine{}
	return p
}

// requestUpdate overwrites the focused recipe with the draft.
//
// This is the only overwrite the tab offers, and it names its target: the user
// is pointing at the recipe being replaced. Saving under an existing name is
// refused instead, because there the collision is a mistake rather than a
// choice.
func (p TunePanel) requestUpdate() (TunePanel, Action) {
	r, ok := p.focusedRecipe()
	if !ok {
		return p.noteRecipeErr(errNoRecipe), actionNone
	}
	if r.Invalid != nil {
		return p.noteRecipeErr(errRecipeUnreadable), actionNone
	}
	return p.emit(RecipeRequest{Op: RecipeUpdate, ID: r.ID, Name: r.Name, Settings: cloneSettings(p.draft)})
}

// requestDelete removes the focused recipe. An unreadable row is deletable on
// purpose: it is the only way the user has of clearing one.
func (p TunePanel) requestDelete() (TunePanel, Action) {
	r, ok := p.focusedRecipe()
	if !ok {
		return p.noteRecipeErr(errNoRecipe), actionNone
	}
	return p.emit(RecipeRequest{Op: RecipeDelete, ID: r.ID, Name: r.Name})
}

// emit records a request and asks the root model to perform it.
func (p TunePanel) emit(req RecipeRequest) (TunePanel, Action) {
	p.request = req
	p.recipeNote = StatusLine{}
	return p, ActionRecipe
}

// noteRecipeErr reports a refusal the panel decided on its own.
func (p TunePanel) noteRecipeErr(err error) TunePanel {
	p.recipeNote = StatusLine{Text: err.Error(), Tone: ToneError}
	return p
}

// handleRecipesKey applies one key press on the Recipes page.
func (p TunePanel) handleRecipesKey(key string) (TunePanel, Action) {
	switch key {
	case "up", "k":
		return p.moveRecipeCursor(-1), actionNone
	case "down", "j":
		return p.moveRecipeCursor(1), actionNone
	case "enter":
		return p.loadFocusedRecipe(), actionNone
	case "s":
		return p.openSavePrompt(), actionNone
	case "u":
		return p.requestUpdate()
	case "n":
		return p.openRenamePrompt(), actionNone
	case "d":
		return p.requestDelete()
	}
	return p, actionNone
}

// handlePromptKey owns every key while the name box is open.
//
// It is checked before the panel-wide keys, so esc closes the box rather than
// the overlay and ctrl+s cannot commit a draft from under a half-typed name.
func (p TunePanel) handlePromptKey(key string) (TunePanel, Action) {
	prompt := *p.prompt
	switch key {
	case "esc":
		p.prompt = nil
		return p, actionNone
	case "enter":
		return p.submitPrompt(prompt)
	case "backspace":
		prompt.value = dropLastRune(prompt.value)
	case "ctrl+u":
		prompt.value = ""
	case "space":
		// A key press reports the space bar by name rather than as " ", and a
		// recipe called "Late Night" is the ordinary case rather than the exotic
		// one, so the name is translated back into the character here.
		prompt.value += " "
	default:
		if runes := []rune(key); len(runes) == 1 {
			prompt.value += key
		}
	}
	p.prompt = &prompt
	return p, actionNone
}

// submitPrompt validates the typed name and emits the request behind it.
func (p TunePanel) submitPrompt(prompt namePrompt) (TunePanel, Action) {
	name := strings.TrimSpace(prompt.value)
	if name == "" {
		return p.noteRecipeErr(errRecipeNameRequired), actionNone
	}
	if p.nameTaken(name, prompt.id) {
		return p.noteRecipeErr(fmt.Errorf("%w: %s", ErrRecipeNameTaken, quoted(name))), actionNone
	}
	p.prompt = nil
	return p.emit(RecipeRequest{Op: prompt.op, ID: prompt.id, Name: name, Settings: prompt.settings})
}

// nameTaken reports whether name already belongs to a recipe other than skipID.
//
// The comparison is case-insensitive because storage's uniqueness index is, so
// refusing here and refusing there mean exactly the same thing. Storage remains
// the authority: this check only spares the user a round trip for a collision
// that is already on screen.
func (p TunePanel) nameTaken(name string, skipID int64) bool {
	folded := strings.ToLower(name)
	for _, r := range p.recipes {
		if r.ID == skipID {
			continue
		}
		if strings.ToLower(r.Name) == folded {
			return true
		}
	}
	return false
}

// clampIndex keeps a cursor inside a list that may have shrunk under it.
func clampIndex(idx, length int) int {
	if length == 0 {
		return 0
	}
	return min(max(idx, 0), length-1)
}

// dropLastRune removes the final rune of s, which is what backspace means for
// text that may hold multi-byte characters.
func dropLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	return string(runes[:len(runes)-1])
}
