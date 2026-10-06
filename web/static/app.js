// coffee (g)rate — small progressive enhancements. The app works without JS.
(() => {
  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => [...el.querySelectorAll(s)];
  const num = (v) => {
    const n = parseFloat(String(v ?? "").replace(",", "."));
    return Number.isFinite(n) ? n : 0;
  };
  const fmt = (n) => (Math.round(n * 10) / 10).toString();

  // ---- flash toast ----
  const flash = $("[data-flash]");
  if (flash) {
    setTimeout(() => flash.classList.add("out"), 4200);
    setTimeout(() => flash.remove(), 4700);
    flash.addEventListener("click", () => flash.remove());
  }

  // ---- confirm destructive forms ----
  $$("form[data-confirm]").forEach((f) =>
    f.addEventListener("submit", (e) => {
      if (!confirm(f.dataset.confirm)) e.preventDefault();
    })
  );

  // ---- auto-submit (sort pills) ----
  $$("[data-autosubmit]").forEach((el) => el.addEventListener("change", () => el.form.submit()));

  // ---- toggles ----
  $$("[data-toggle]").forEach((cb) => {
    const target = $(cb.dataset.toggle);
    if (!target) return;
    const sync = () => (target.hidden = !cb.checked);
    cb.addEventListener("change", sync);
    sync();
  });

  // ---- show field only for a given radio value (e.g. portafilter for espresso) ----
  $$("[data-show-when]").forEach((el) => {
    const [name, value] = el.dataset.showWhen.split("=");
    const form = el.closest("form");
    const sync = () => {
      const checked = form.querySelector(`[name="${name}"]:checked`);
      el.hidden = !(checked && checked.value.toLowerCase() === value.toLowerCase());
    };
    form.addEventListener("change", sync);
    sync();
  });

  // ---- range outputs ----
  $$('input[type="range"]').forEach((r) => {
    const out = r.form && r.form.querySelector(`[data-out="${r.name}"]`);
    if (!out) return;
    r.addEventListener("input", () => (out.textContent = fmt(num(r.value))));
  });

  // ---- live flavour radar ----
  $$("[data-radar]").forEach((svg) => {
    const form = svg.closest("form");
    if (!form) return;
    const keys = svg.dataset.keys.split(",");
    const cx = num(svg.dataset.cx), cy = num(svg.dataset.cy), R = num(svg.dataset.r);
    const shape = $(".radar-shape", svg);
    const dots = $$(".radar-dot", svg);
    const pt = (i, v) => {
      const a = (i * 2 * Math.PI) / keys.length - Math.PI / 2;
      return [cx + Math.cos(a) * R * v / 10, cy + Math.sin(a) * R * v / 10];
    };
    const draw = () => {
      const pts = keys.map((k, i) => {
        const input = form.querySelector(`[name="${k}"]`);
        const v = input ? num(input.value) : 0;
        const [x, y] = pt(i, v);
        if (dots[i]) { dots[i].setAttribute("cx", x.toFixed(1)); dots[i].setAttribute("cy", y.toFixed(1)); }
        const label = svg.querySelector(`[data-val="${k}"]`);
        if (label) label.textContent = v;
        return `${x.toFixed(1)},${y.toFixed(1)}`;
      });
      shape.setAttribute("points", pts.join(" "));
    };
    form.addEventListener("input", (e) => { if (e.target.matches("[data-radar-input]")) draw(); });
    draw();
  });

  // ---- flavour chips ----
  const flavorInput = $("#flavor_notes");
  if (flavorInput) {
    const list = () => flavorInput.value.split(",").map((s) => s.trim()).filter(Boolean);
    const sync = () => {
      const cur = list().map((s) => s.toLowerCase());
      $$("[data-add-flavor]").forEach((b) => b.classList.toggle("on", cur.includes(b.dataset.addFlavor.toLowerCase())));
    };
    $$("[data-add-flavor]").forEach((b) =>
      b.addEventListener("click", () => {
        const f = b.dataset.addFlavor;
        let cur = list();
        const i = cur.findIndex((s) => s.toLowerCase() === f.toLowerCase());
        if (i >= 0) cur.splice(i, 1); else cur.push(f.toLowerCase());
        flavorInput.value = cur.join(", ");
        sync();
      })
    );
    flavorInput.addEventListener("input", sync);
    sync();
  }

  // ---- ratio / flow (recipe & shot forms) ----
  $$("[data-ratio-form]").forEach((form) => {
    const dose = $("[data-dose]", form), yieldEl = $("[data-yield]", form), time = $("[data-time]", form);
    const out = $("[data-ratio]", form), nameEl = $("[data-ratio-name]", form);
    const bar = $("[data-ratio-bar]", form), flow = $("[data-flow]", form);
    const methodSel = $("[data-method-select]", form);
    const method = () => {
      const o = methodSel && methodSel.selectedOptions[0];
      return ((o && o.dataset.method) || "").toLowerCase();
    };
    const ratioName = (r) => {
      if (r <= 0) return "";
      const m = method();
      if (m && m !== "espresso") return r < 12 ? "strong" : r <= 17 ? "classic" : "light";
      return r < 1.5 ? "ristretto" : r <= 2.5 ? "normale" : r <= 4 ? "lungo" : "allongé";
    };
    const update = () => {
      const d = num(dose && dose.value), y = num(yieldEl && yieldEl.value), t = num(time && time.value);
      const r = d > 0 ? y / d : 0;
      if (out) out.textContent = r > 0 ? `1:${r.toFixed(1)}` : "–";
      if (nameEl) nameEl.textContent = ratioName(r);
      if (bar) bar.style.width = d + y > 0 ? `${(d / (d + y)) * 100}%` : "0";
      if (flow) flow.textContent = y > 0 && t > 0 ? `${(y / t).toFixed(1)} g/s` : "–";
    };
    form.addEventListener("input", update);
    form.addEventListener("change", update);
    $$("[data-ratio-preset]", form).forEach((b) =>
      b.addEventListener("click", () => {
        const d = num(dose.value);
        if (d > 0) { yieldEl.value = fmt(d * num(b.dataset.ratioPreset)); update(); }
        else dose.focus();
      })
    );
    update();
  });

  // ---- shot form: apply recipe, taste hint, timer ----
  const shotForm = $("[data-shot-form]");
  if (shotForm) {
    const set = (name, v) => {
      const el = shotForm.querySelector(`[name="${name}"]`);
      if (el && v !== undefined && v !== "") el.value = v;
    };
    $$('input[name="recipe_id"]', shotForm).forEach((rb) =>
      rb.addEventListener("change", () => {
        if (!rb.checked || !rb.value) return;
        const d = rb.dataset;
        set("dose_g", d.recDose); set("yield_g", d.recYield); set("time_s", d.recTime); set("temp_c", d.recTemp);
        set("grind_setting", d.recGrind); set("brewer_id", d.recBrewer); set("grinder_id", d.recGrinder); set("bean_id", d.recBean);
        shotForm.dispatchEvent(new Event("input"));
      })
    );

    const hint = $("[data-taste-hint]", shotForm);
    const hints = {
      "-2": "Very sour → grind finer, raise the temperature or pull a longer ratio.",
      "-1": "A touch sour → grind a notch finer or go slightly longer.",
      "0": "Balanced. Dialed in — keep these numbers.",
      "1": "A touch bitter → grind a notch coarser or pull a bit shorter.",
      "2": "Very bitter → grind coarser, lower the temperature or shorten the ratio.",
    };
    $$('input[name="taste"]', shotForm).forEach((rb) =>
      rb.addEventListener("change", () => { if (hint && rb.checked) hint.textContent = hints[rb.value]; })
    );

    const timerBtn = $("[data-timer]", shotForm);
    const timeInput = $("[data-time]", shotForm);
    if (timerBtn && timeInput) {
      const label = $("[data-timer-label]", timerBtn);
      let start = 0, raf = 0;
      const tick = () => {
        timeInput.value = Math.floor((performance.now() - start) / 1000);
        shotForm.dispatchEvent(new Event("input"));
        raf = requestAnimationFrame(tick);
      };
      timerBtn.addEventListener("click", () => {
        if (start) {
          cancelAnimationFrame(raf);
          timeInput.value = Math.round((performance.now() - start) / 1000);
          shotForm.dispatchEvent(new Event("input"));
          start = 0;
          timerBtn.classList.remove("running");
          label.textContent = "Restart";
          if (navigator.vibrate) navigator.vibrate(30);
        } else {
          start = performance.now();
          timerBtn.classList.add("running");
          label.textContent = "Stop";
          if (navigator.vibrate) navigator.vibrate(15);
          tick();
        }
      });
    }
  }
})();
