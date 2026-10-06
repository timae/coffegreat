-- Gear ---------------------------------------------------------------

CREATE TABLE grinders (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    brand         TEXT NOT NULL DEFAULT '',
    burr_type     TEXT NOT NULL DEFAULT '',
    burr_size_mm  INTEGER,
    notes         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE brewers (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    brand           TEXT NOT NULL DEFAULT '',
    method          TEXT NOT NULL DEFAULT 'espresso',
    portafilter_mm  INTEGER,
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Beans --------------------------------------------------------------

CREATE TABLE beans (
    id             BIGSERIAL PRIMARY KEY,
    name           TEXT NOT NULL,
    roaster        TEXT NOT NULL DEFAULT '',
    origin         TEXT NOT NULL DEFAULT '',
    region         TEXT NOT NULL DEFAULT '',
    varietal       TEXT NOT NULL DEFAULT '',
    process        TEXT NOT NULL DEFAULT '',
    roast_level    TEXT NOT NULL DEFAULT '',
    is_blend       BOOLEAN NOT NULL DEFAULT false,
    roast_date     DATE,
    weight_g       INTEGER,
    price          DOUBLE PRECISION,
    flavor_notes   TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    -- tasting scores, 0..10 (NULL = not rated yet)
    score_overall  DOUBLE PRECISION,
    aroma          SMALLINT,
    acidity        SMALLINT,
    sweetness      SMALLINT,
    body           SMALLINT,
    bitterness     SMALLINT,
    aftertaste     SMALLINT,
    balance        SMALLINT,
    buy_again      BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Recipes (portafilter / brew profiles) ------------------------------

CREATE TABLE recipes (
    id             BIGSERIAL PRIMARY KEY,
    name           TEXT NOT NULL,
    brewer_id      BIGINT REFERENCES brewers(id) ON DELETE SET NULL,
    grinder_id     BIGINT REFERENCES grinders(id) ON DELETE SET NULL,
    bean_id        BIGINT REFERENCES beans(id) ON DELETE SET NULL,
    basket         TEXT NOT NULL DEFAULT '',
    basket_g       DOUBLE PRECISION,
    dose_g         DOUBLE PRECISION NOT NULL,
    yield_g        DOUBLE PRECISION NOT NULL,
    time_s         INTEGER,
    temp_c         DOUBLE PRECISION,
    grind_setting  TEXT NOT NULL DEFAULT '',
    preinfusion_s  INTEGER,
    pressure_bar   DOUBLE PRECISION,
    prep           TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Shots / brews ------------------------------------------------------

CREATE TABLE shots (
    id             BIGSERIAL PRIMARY KEY,
    bean_id        BIGINT REFERENCES beans(id) ON DELETE SET NULL,
    recipe_id      BIGINT REFERENCES recipes(id) ON DELETE SET NULL,
    brewer_id      BIGINT REFERENCES brewers(id) ON DELETE SET NULL,
    grinder_id     BIGINT REFERENCES grinders(id) ON DELETE SET NULL,
    grind_setting  TEXT NOT NULL DEFAULT '',
    dose_g         DOUBLE PRECISION,
    yield_g        DOUBLE PRECISION,
    time_s         INTEGER,
    temp_c         DOUBLE PRECISION,
    rating         SMALLINT CHECK (rating BETWEEN 1 AND 5),
    taste          SMALLINT CHECK (taste BETWEEN -2 AND 2),
    notes          TEXT NOT NULL DEFAULT '',
    brewed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX shots_bean_idx      ON shots (bean_id);
CREATE INDEX shots_recipe_idx    ON shots (recipe_id);
CREATE INDEX shots_brewed_at_idx ON shots (brewed_at DESC);
