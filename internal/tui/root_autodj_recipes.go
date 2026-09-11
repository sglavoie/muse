package tui

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/autodj"
)

// This file connects the Recipes tab to storage.
//
// What crosses the boundary is deliberately narrow. The panel asks for four
// durable things — save, update, rename, delete — and reads the list back after
// each. Loading a recipe is not here at all: it edits the draft and writes
// nothing, so the panel does it alone, and no route through this file can reach
// the session, the queue, or the player. That is what makes "no implicit
// playback changes from Save or Load" a property of the wiring rather than a
// promise about the handlers.
//
// Storage is the authority on name collisions. The panel refuses a duplicate it
// can already see, which spares a round trip, but the refusal that counts is the
// unique index underneath: two recipes cannot end up sharing a name because a
// list went stale between the read and the write.

// errDJRecipeGone reports a recipe that vanished between being listed and being
// acted on. The list is reloaded alongside the message, so the user sees the row
// disappear rather than being told about a row still on screen.
var errDJRecipeGone = errors.New("that recipe no longer exists")

// errNoDJRecipeStore reports a build with no recipe storage wired in.
var errNoDJRecipeStore = errors.New("recipes are unavailable without a library database")

// autoDJRecipes reads the saved recipes for the Recipes tab.
func (m Model) autoDJRecipes() ([]autodj.Recipe, error) {
	if m.djRecipes == nil {
		return nil, nil
	}
	rows, err := m.djRecipes.ReadRecipes()
	if err != nil {
		return nil, err
	}
	out := make([]autodj.Recipe, 0, len(rows))
	for _, row := range rows {
		out = append(out, autodj.Recipe{
			ID:       row.Recipe.ID,
			Name:     row.Recipe.Name,
			Settings: row.Recipe.Settings,
			Invalid:  row.Invalid,
		})
	}
	return out, nil
}

// performAutoDJRecipe carries out one recipe request and reports the outcome.
//
// The writes are synchronous, like the session writes in root_dj_session.go.
// They are single-row statements against a local database, and doing them on the
// update loop is what lets the reloaded list and the notice about it arrive in
// the same frame — an asynchronous save would have to reconcile a list the user
// may already have moved the cursor through.
func (m Model) performAutoDJRecipe(req autodj.RecipeRequest) autodj.RecipeOutcome {
	res := autodj.RecipeOutcome{Op: req.Op, Name: req.Name}
	if m.djRecipeWriter == nil {
		res.Err = errNoDJRecipeStore
		return res
	}
	res.Err = djRecipeErr(m.writeAutoDJRecipe(req))

	// The list is re-read whether or not the write succeeded. A refused save
	// still means the list on screen may be older than storage — that is often
	// exactly why it was refused — and showing the name that collided is what
	// makes the refusal actionable.
	recipes, err := m.autoDJRecipes()
	if err != nil {
		slog.Warn("autodj: reload recipes", "err", err)
		return res
	}
	res.Recipes = recipes
	return res
}

// writeAutoDJRecipe performs the durable half of one request.
func (m Model) writeAutoDJRecipe(req autodj.RecipeRequest) error {
	switch req.Op {
	case autodj.RecipeSave:
		_, err := m.djRecipeWriter.CreateRecipe(radio.Recipe{Name: req.Name, Settings: req.Settings})
		return err
	case autodj.RecipeUpdate, autodj.RecipeRename:
		// Both go through UpdateRecipe because storage rewrites the whole row.
		// The panel is what decides which settings ride along: the draft for an
		// update, the recipe's own stored settings for a rename. Deciding it
		// here instead would make renaming a recipe silently re-save it.
		return m.djRecipeWriter.UpdateRecipe(radio.Recipe{
			ID: req.ID, Name: req.Name, Settings: req.Settings})
	case autodj.RecipeDelete:
		return m.djRecipeWriter.DeleteRecipeSoft(req.ID)
	}
	return fmt.Errorf("unknown recipe request %d", req.Op)
}

// djRecipeErr translates a storage failure into what the panel should say.
//
// The two named ones are the ones the user can act on, and both are restated
// without storage's own prefix: a collision is the refusal the tab is built
// around, and a missing row is a list that moved under the cursor.
func djRecipeErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, db.ErrRecipeNameTaken):
		return autodj.ErrRecipeNameTaken
	case errors.Is(err, db.ErrRecipeNotFound):
		return errDJRecipeGone
	}
	return err
}
