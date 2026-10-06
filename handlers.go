package main

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"
)

func (app *App) routes() http.Handler {
	mux := http.NewServeMux()

	static, _ := fs.Sub(webFS, "web/static")
	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(static)))
	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /healthz", app.healthz)

	mux.HandleFunc("GET /login", app.loginPage)
	mux.HandleFunc("POST /login", app.loginSubmit)
	mux.HandleFunc("POST /logout", app.logout)

	mux.HandleFunc("GET /{$}", app.dashboard)

	mux.HandleFunc("GET /beans", app.beansIndex)
	mux.HandleFunc("GET /beans/new", app.beanNew)
	mux.HandleFunc("POST /beans", app.beanSave)
	mux.HandleFunc("GET /beans/{id}", app.beanShow)
	mux.HandleFunc("GET /beans/{id}/edit", app.beanEdit)
	mux.HandleFunc("POST /beans/{id}", app.beanSave)
	mux.HandleFunc("POST /beans/{id}/delete", app.del("beans", "/beans", "Bean removed."))

	mux.HandleFunc("GET /gear", app.gearIndex)
	mux.HandleFunc("GET /gear/grinders/new", app.grinderForm)
	mux.HandleFunc("POST /gear/grinders", app.grinderSave)
	mux.HandleFunc("GET /gear/grinders/{id}/edit", app.grinderForm)
	mux.HandleFunc("POST /gear/grinders/{id}", app.grinderSave)
	mux.HandleFunc("POST /gear/grinders/{id}/delete", app.del("grinders", "/gear", "Grinder removed."))
	mux.HandleFunc("GET /gear/brewers/new", app.brewerForm)
	mux.HandleFunc("POST /gear/brewers", app.brewerSave)
	mux.HandleFunc("GET /gear/brewers/{id}/edit", app.brewerForm)
	mux.HandleFunc("POST /gear/brewers/{id}", app.brewerSave)
	mux.HandleFunc("POST /gear/brewers/{id}/delete", app.del("brewers", "/gear", "Brewer removed."))

	mux.HandleFunc("GET /recipes", app.recipesIndex)
	mux.HandleFunc("GET /recipes/new", app.recipeForm)
	mux.HandleFunc("POST /recipes", app.recipeSave)
	mux.HandleFunc("GET /recipes/{id}/edit", app.recipeForm)
	mux.HandleFunc("POST /recipes/{id}", app.recipeSave)
	mux.HandleFunc("POST /recipes/{id}/delete", app.del("recipes", "/recipes", "Recipe removed."))

	mux.HandleFunc("GET /shots", app.shotsIndex)
	mux.HandleFunc("GET /shots/new", app.shotForm)
	mux.HandleFunc("POST /shots", app.shotSave)
	mux.HandleFunc("GET /shots/{id}/edit", app.shotForm)
	mux.HandleFunc("POST /shots/{id}", app.shotSave)
	mux.HandleFunc("POST /shots/{id}/delete", app.del("shots", "/shots", "Shot deleted."))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		app.render(w, r, http.StatusNotFound, "notfound", V{"Title": "Not found"})
	})

	return app.middleware(mux)
}

// ---------- helpers ----------

func (app *App) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		app.render(w, r, http.StatusNotFound, "notfound", V{"Title": "Not found"})
		return
	}
	log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	http.Error(w, "Something went wrong. Check the logs.", http.StatusInternalServerError)
}

func ctx(r *http.Request) context.Context { return r.Context() }

func (app *App) del(table, back, msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(r)
		if !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
		if err := deleteRow(ctx(r), app.db, table, id); err != nil {
			app.fail(w, r, err)
			return
		}
		setFlash(w, msg)
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

func redirectFlash(w http.ResponseWriter, r *http.Request, to, msg string) {
	setFlash(w, msg)
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// ---------- system ----------

func (app *App) healthz(w http.ResponseWriter, r *http.Request) {
	c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := app.db.PingContext(c); err != nil {
		http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

func safeNext(n string) string {
	if !strings.HasPrefix(n, "/") || strings.HasPrefix(n, "//") || strings.HasPrefix(n, "/\\") {
		return "/"
	}
	return n
}

func (app *App) loginPage(w http.ResponseWriter, r *http.Request) {
	if app.auth.valid(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	app.render(w, r, http.StatusOK, "login", V{"Title": "Sign in", "Next": safeNext(r.URL.Query().Get("next")), "Art": loginArt()})
}

// loginArt: a fresh generative label every day.
func loginArt() any {
	day := time.Now().Format("2006-01-02")
	roasts := []string{"light", "medium-light", "medium", "medium-dark"}
	procs := []string{"washed", "natural", "honey", "anaerobic"}
	d := time.Now().YearDay()
	return labelArt(Bean{Name: day, RoastLevel: roasts[d%len(roasts)], Process: procs[(d/4)%len(procs)]})
}

func (app *App) loginSubmit(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.FormValue("next"))
	if app.auth.disabled || app.auth.check(r.FormValue("password")) {
		if !app.auth.disabled {
			app.auth.issue(w, r)
		}
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	time.Sleep(700 * time.Millisecond) // slow down guessing
	app.render(w, r, http.StatusUnauthorized, "login", V{"Title": "Sign in", "Next": next, "Art": loginArt(), "Error": "That's not it. Try again."})
}

func (app *App) logout(w http.ResponseWriter, r *http.Request) {
	app.auth.clear(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ---------- dashboard ----------

func (app *App) dashboard(w http.ResponseWriter, r *http.Request) {
	c := ctx(r)
	stats, err := loadStats(c, app.db)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	shots, err := listShots(c, app.db, ShotFilter{Limit: 30})
	if err != nil {
		app.fail(w, r, err)
		return
	}
	recipes, err := listRecipes(c, app.db,
		"(SELECT COUNT(*) FROM shots s WHERE s.recipe_id = r.id) DESC, lower(r.name)")
	if err != nil {
		app.fail(w, r, err)
		return
	}
	if len(recipes) > 4 {
		recipes = recipes[:4]
	}
	top, err := listBeans(c, app.db, "", "score", 4)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	flavors, _ := allFlavorNotes(c, app.db)
	recent := shots
	if len(recent) > 6 {
		recent = recent[:6]
	}
	app.render(w, r, http.StatusOK, "dashboard", V{
		"Title": "Today", "Nav": "home",
		"Stats": stats, "Pulse": shots, "Recent": recent, "Recipes": recipes, "TopBeans": top, "Flavors": flavors,
		"SetupDone": stats.Grinders > 0 && stats.Brewers > 0 && stats.Beans > 0 && stats.Recipes > 0,
	})
}

// ---------- beans ----------

func (app *App) beansIndex(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sortBy := r.URL.Query().Get("sort")
	beans, err := listBeans(ctx(r), app.db, q, sortBy, 0)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	app.render(w, r, http.StatusOK, "beans", V{"Title": "Beans", "Nav": "beans", "Beans": beans, "Q": q, "Sort": sortBy})
}

func (app *App) beanShow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		app.fail(w, r, sql.ErrNoRows)
		return
	}
	b, err := getBean(ctx(r), app.db, id)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	shots, err := listShots(ctx(r), app.db, ShotFilter{BeanID: id, Limit: 40})
	if err != nil {
		app.fail(w, r, err)
		return
	}
	all, err := listRecipes(ctx(r), app.db, "")
	if err != nil {
		app.fail(w, r, err)
		return
	}
	var recipes []Recipe
	for _, rc := range all {
		if rc.BeanID != nil && *rc.BeanID == id {
			recipes = append(recipes, rc)
		}
	}
	app.render(w, r, http.StatusOK, "bean", V{"Title": b.Name, "Nav": "beans", "Bean": b, "Shots": shots, "Recipes": recipes})
}

func (app *App) beanNew(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusOK, "bean_form", V{"Title": "New bean", "Nav": "beans", "Bean": Bean{}, "IsNew": true})
}

func (app *App) beanEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		app.fail(w, r, sql.ErrNoRows)
		return
	}
	b, err := getBean(ctx(r), app.db, id)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	app.render(w, r, http.StatusOK, "bean_form", V{"Title": "Edit " + b.Name, "Nav": "beans", "Bean": b})
}

func normalizeFlavors(s string) string {
	seen := map[string]bool{}
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" || len(f) > 40 || seen[strings.ToLower(f)] {
			continue
		}
		seen[strings.ToLower(f)] = true
		out = append(out, f)
		if len(out) == 12 {
			break
		}
	}
	return strings.Join(out, ", ")
}

func (app *App) beanSave(w http.ResponseWriter, r *http.Request) {
	var id int64
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(r); !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
	}
	b := Bean{
		ID:          id,
		Name:        fStrMax(r, "name", 120),
		Roaster:     fStrMax(r, "roaster", 120),
		Origin:      fStrMax(r, "origin", 120),
		Region:      fStrMax(r, "region", 120),
		Varietal:    fStrMax(r, "varietal", 120),
		Process:     oneOf(fStr(r, "process"), Processes),
		RoastLevel:  oneOf(fStr(r, "roast_level"), RoastLevels),
		IsBlend:     fBool(r, "is_blend"),
		RoastDate:   fDate(r, "roast_date"),
		WeightG:     fInt(r, "weight_g"),
		Price:       fFloat(r, "price"),
		FlavorNotes: normalizeFlavors(fStrMax(r, "flavor_notes", 600)),
		Notes:       fStrMax(r, "notes", 4000),
		BuyAgain:    fBool(r, "buy_again"),
	}
	if fBool(r, "rated") {
		b.Overall = fFloat(r, "overall")
		if b.Overall != nil && *b.Overall > 10 {
			ten := 10.0
			b.Overall = &ten
		}
		b.Aroma = fRange(r, "aroma", 0, 10)
		b.Acidity = fRange(r, "acidity", 0, 10)
		b.Sweetness = fRange(r, "sweetness", 0, 10)
		b.Body = fRange(r, "body", 0, 10)
		b.Bitterness = fRange(r, "bitterness", 0, 10)
		b.Aftertaste = fRange(r, "aftertaste", 0, 10)
		b.Balance = fRange(r, "balance", 0, 10)
	}
	if b.Name == "" {
		app.render(w, r, http.StatusUnprocessableEntity, "bean_form", V{"Title": "Bean", "Nav": "beans", "Bean": b,
			"IsNew": id == 0, "Error": "Give this coffee a name."})
		return
	}
	if err := saveBean(ctx(r), app.db, &b); err != nil {
		app.fail(w, r, err)
		return
	}
	msg := "Bean saved."
	if id == 0 {
		msg = "Welcome to the shelf, " + b.Name + "."
	}
	redirectFlash(w, r, "/beans/"+itoa(int(b.ID)), msg)
}

// ---------- gear ----------

func (app *App) gearIndex(w http.ResponseWriter, r *http.Request) {
	g, err := listGrinders(ctx(r), app.db)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	b, err := listBrewers(ctx(r), app.db)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	app.render(w, r, http.StatusOK, "gear", V{"Title": "Gear", "Nav": "gear", "Grinders": g, "Brewers": b})
}

func (app *App) grinderForm(w http.ResponseWriter, r *http.Request) {
	g := Grinder{}
	if r.PathValue("id") != "" {
		id, ok := pathID(r)
		if !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
		var err error
		if g, err = getGrinder(ctx(r), app.db, id); err != nil {
			app.fail(w, r, err)
			return
		}
	}
	app.render(w, r, http.StatusOK, "grinder_form", V{"Title": "Grinder", "Nav": "gear", "Grinder": g, "IsNew": g.ID == 0})
}

func (app *App) grinderSave(w http.ResponseWriter, r *http.Request) {
	var id int64
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(r); !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
	}
	g := Grinder{ID: id, Name: fStrMax(r, "name", 120), Brand: fStrMax(r, "brand", 120),
		BurrType: oneOf(fStr(r, "burr_type"), BurrTypes), BurrSizeMM: fInt(r, "burr_size_mm"), Notes: fStrMax(r, "notes", 2000)}
	if g.Name == "" {
		app.render(w, r, http.StatusUnprocessableEntity, "grinder_form", V{"Title": "Grinder", "Nav": "gear", "Grinder": g,
			"IsNew": id == 0, "Error": "Your grinder needs a name."})
		return
	}
	if err := saveGrinder(ctx(r), app.db, &g); err != nil {
		app.fail(w, r, err)
		return
	}
	redirectFlash(w, r, "/gear", g.Name+" saved.")
}

func (app *App) brewerForm(w http.ResponseWriter, r *http.Request) {
	b := Brewer{Method: "Espresso"}
	if r.PathValue("id") != "" {
		id, ok := pathID(r)
		if !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
		var err error
		if b, err = getBrewer(ctx(r), app.db, id); err != nil {
			app.fail(w, r, err)
			return
		}
	}
	app.render(w, r, http.StatusOK, "brewer_form", V{"Title": "Brewer", "Nav": "gear", "Brewer": b, "IsNew": b.ID == 0})
}

func (app *App) brewerSave(w http.ResponseWriter, r *http.Request) {
	var id int64
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(r); !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
	}
	b := Brewer{ID: id, Name: fStrMax(r, "name", 120), Brand: fStrMax(r, "brand", 120),
		Method: oneOf(fStr(r, "method"), BrewMethods), PortafilterMM: fInt(r, "portafilter_mm"), Notes: fStrMax(r, "notes", 2000)}
	if b.Method == "" {
		b.Method = "Espresso"
	}
	if !b.IsEspresso() {
		b.PortafilterMM = nil
	}
	if b.Name == "" {
		app.render(w, r, http.StatusUnprocessableEntity, "brewer_form", V{"Title": "Brewer", "Nav": "gear", "Brewer": b,
			"IsNew": id == 0, "Error": "Your machine needs a name."})
		return
	}
	if err := saveBrewer(ctx(r), app.db, &b); err != nil {
		app.fail(w, r, err)
		return
	}
	redirectFlash(w, r, "/gear", b.Name+" saved.")
}

// ---------- recipes ----------

func (app *App) recipesIndex(w http.ResponseWriter, r *http.Request) {
	recipes, err := listRecipes(ctx(r), app.db, "")
	if err != nil {
		app.fail(w, r, err)
		return
	}
	app.render(w, r, http.StatusOK, "recipes", V{"Title": "Recipes", "Nav": "recipes", "Recipes": recipes})
}

func (app *App) pickers(c context.Context, v V) error {
	brewers, err := listBrewers(c, app.db)
	if err != nil {
		return err
	}
	grinders, err := listGrinders(c, app.db)
	if err != nil {
		return err
	}
	beans, err := listBeans(c, app.db, "", "name", 0)
	if err != nil {
		return err
	}
	v["Brewers"], v["Grinders"], v["Beans"] = brewers, grinders, beans
	return nil
}

func (app *App) recipeForm(w http.ResponseWriter, r *http.Request) {
	rc := Recipe{DoseG: 18, YieldG: 36}
	if r.PathValue("id") != "" {
		id, ok := pathID(r)
		if !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
		var err error
		if rc, err = getRecipe(ctx(r), app.db, id); err != nil {
			app.fail(w, r, err)
			return
		}
	}
	v := V{"Title": "Recipe", "Nav": "recipes", "Recipe": rc, "IsNew": rc.ID == 0}
	if err := app.pickers(ctx(r), v); err != nil {
		app.fail(w, r, err)
		return
	}
	app.render(w, r, http.StatusOK, "recipe_form", v)
}

func (app *App) recipeSave(w http.ResponseWriter, r *http.Request) {
	var id int64
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(r); !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
	}
	rc := Recipe{ID: id, Name: fStrMax(r, "name", 120), BrewerID: fID(r, "brewer_id"), GrinderID: fID(r, "grinder_id"),
		BeanID: fID(r, "bean_id"), Basket: fStrMax(r, "basket", 120), BasketG: fFloat(r, "basket_g"),
		TimeS: fInt(r, "time_s"), TempC: fFloat(r, "temp_c"), GrindSetting: fStrMax(r, "grind_setting", 60),
		PreinfusionS: fInt(r, "preinfusion_s"), PressureBar: fFloat(r, "pressure_bar"),
		Prep: fStrMax(r, "prep", 600), Notes: fStrMax(r, "notes", 4000)}
	if d := fFloat(r, "dose_g"); d != nil {
		rc.DoseG = *d
	}
	if y := fFloat(r, "yield_g"); y != nil {
		rc.YieldG = *y
	}
	var problem string
	switch {
	case rc.Name == "":
		problem = "Name your recipe — e.g. “Daily double” or “Fruity light roast”."
	case rc.DoseG <= 0 || rc.YieldG <= 0:
		problem = "Dose and yield are the heart of a recipe — both need a value."
	}
	if problem != "" {
		v := V{"Title": "Recipe", "Nav": "recipes", "Recipe": rc, "IsNew": id == 0, "Error": problem}
		if err := app.pickers(ctx(r), v); err != nil {
			app.fail(w, r, err)
			return
		}
		app.render(w, r, http.StatusUnprocessableEntity, "recipe_form", v)
		return
	}
	if err := saveRecipe(ctx(r), app.db, &rc); err != nil {
		app.fail(w, r, err)
		return
	}
	redirectFlash(w, r, "/recipes", "Recipe “"+rc.Name+"” saved.")
}

// ---------- shots ----------

type DayGroup struct {
	Label string
	Shots []Shot
}

func (app *App) shotsIndex(w http.ResponseWriter, r *http.Request) {
	beanID := queryID(r, "bean")
	shots, err := listShots(ctx(r), app.db, ShotFilter{BeanID: beanID, Limit: 300})
	if err != nil {
		app.fail(w, r, err)
		return
	}
	var groups []DayGroup
	for _, s := range shots {
		label := dayLabel(s.BrewedAt)
		if len(groups) == 0 || groups[len(groups)-1].Label != label {
			groups = append(groups, DayGroup{Label: label})
		}
		groups[len(groups)-1].Shots = append(groups[len(groups)-1].Shots, s)
	}
	beans, err := listBeans(ctx(r), app.db, "", "brewed", 0)
	if err != nil {
		app.fail(w, r, err)
		return
	}
	var withShots []Bean
	for _, b := range beans {
		if b.ShotCount > 0 {
			withShots = append(withShots, b)
		}
	}
	app.render(w, r, http.StatusOK, "shots", V{"Title": "Shots", "Nav": "shots", "Groups": groups, "Count": len(shots),
		"Beans": withShots, "BeanID": beanID, "Pulse": shots})
}

func (app *App) shotForm(w http.ResponseWriter, r *http.Request) {
	c := ctx(r)
	s := Shot{BrewedAt: time.Now()}
	if r.PathValue("id") != "" {
		id, ok := pathID(r)
		if !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
		var err error
		if s, err = getShot(c, app.db, id); err != nil {
			app.fail(w, r, err)
			return
		}
	} else {
		// Sensible defaults: copy a shot, apply a recipe, or continue where you left off.
		if again := queryID(r, "again"); again > 0 {
			if prev, err := getShot(c, app.db, again); err == nil {
				s = prev
				s.ID, s.Rating, s.Taste, s.Notes, s.BrewedAt = 0, nil, nil, "", time.Now()
			}
		} else if last, ok := lastShot(c, app.db); ok {
			s.BeanID, s.BrewerID, s.GrinderID, s.RecipeID = last.BeanID, last.BrewerID, last.GrinderID, last.RecipeID
			s.GrindSetting, s.DoseG, s.YieldG, s.TimeS, s.TempC = last.GrindSetting, last.DoseG, last.YieldG, last.TimeS, last.TempC
		}
		if rid := queryID(r, "recipe"); rid > 0 {
			if rc, err := getRecipe(c, app.db, rid); err == nil {
				applyRecipe(&s, rc)
			}
		}
		if bid := queryID(r, "bean"); bid > 0 {
			s.BeanID = &bid
		}
	}
	v := V{"Title": "Log a shot", "Nav": "shots", "Shot": s, "IsNew": s.ID == 0}
	if err := app.pickers(c, v); err != nil {
		app.fail(w, r, err)
		return
	}
	recipes, err := listRecipes(c, app.db, "")
	if err != nil {
		app.fail(w, r, err)
		return
	}
	v["Recipes"] = recipes
	app.render(w, r, http.StatusOK, "shot_form", v)
}

func applyRecipe(s *Shot, rc Recipe) {
	id := rc.ID
	dose, yield := rc.DoseG, rc.YieldG
	s.RecipeID = &id
	s.DoseG, s.YieldG, s.TimeS, s.TempC = &dose, &yield, rc.TimeS, rc.TempC
	if rc.GrindSetting != "" {
		s.GrindSetting = rc.GrindSetting
	}
	if rc.BrewerID != nil {
		s.BrewerID = rc.BrewerID
	}
	if rc.GrinderID != nil {
		s.GrinderID = rc.GrinderID
	}
	if rc.BeanID != nil {
		s.BeanID = rc.BeanID
	}
}

func (app *App) shotSave(w http.ResponseWriter, r *http.Request) {
	var id int64
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(r); !ok {
			app.fail(w, r, sql.ErrNoRows)
			return
		}
	}
	s := Shot{ID: id, BeanID: fID(r, "bean_id"), RecipeID: fID(r, "recipe_id"), BrewerID: fID(r, "brewer_id"),
		GrinderID: fID(r, "grinder_id"), GrindSetting: fStrMax(r, "grind_setting", 60), DoseG: fFloat(r, "dose_g"),
		YieldG: fFloat(r, "yield_g"), TimeS: fInt(r, "time_s"), TempC: fFloat(r, "temp_c"),
		Rating: fRange(r, "rating", 1, 5), Taste: fRange(r, "taste", -2, 2), Notes: fStrMax(r, "notes", 4000),
		BrewedAt: fDateTime(r, "brewed_at")}
	if err := saveShot(ctx(r), app.db, &s); err != nil {
		app.fail(w, r, err)
		return
	}
	msg := "Shot updated."
	if id == 0 {
		msg = "Shot logged."
		if h := tasteHint(s.Taste); h != "" {
			msg += " " + h
		}
	}
	to := "/shots"
	if fStr(r, "return") == "bean" && s.BeanID != nil {
		to = "/beans/" + itoa(int(*s.BeanID))
	}
	redirectFlash(w, r, to, msg)
}
