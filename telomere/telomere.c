/* telomere.c — Core dispatch and lifecycle for the telomere Pd external
 *
 * telomere records rhythmic tap-patterns as fractional positions within a
 * cycle (0.0–1.0) and plays them back, applying registered transforms.
 */

#include "telomere.h"
#include "telomere_transform.h"
#include "telomere_pattern_api.h"

t_class *telomere_class;

/* ------------------------------------------------------------------ */
/* Playback clock callback                                            */
/* ------------------------------------------------------------------ */

static void telomere_tick(t_telomere *x) {
    if (x->play_index >= x->num_events) {
        /* Cycle complete — output event count and stop */
        outlet_float(x->out_count, (t_float)x->num_events);
        return;
    }

    t_float pos = x->pattern[x->play_index];

    /* Apply skip probability */
    if (x->skip_prob > 0.0f) {
        float r = (float)rand() / (float)RAND_MAX;
        if (r < x->skip_prob) {
            /* Skip this event, schedule next */
            x->play_index++;
            if (x->play_index < x->num_events) {
                double next_pos = x->pattern[x->play_index];
                double delay = next_pos * x->cycle_length_ms
                             - (pos * x->cycle_length_ms);
                if (delay < 0.1) delay = 0.1;
                clock_delay(x->playback_clock, delay);
            } else {
                outlet_float(x->out_count, (t_float)x->num_events);
            }
            return;
        }
    }

    /* Apply jitter */
    t_float out_pos = pos;
    if (x->jitter_amt > 0.0f) {
        float r = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
        out_pos += r * x->jitter_amt;
        if (out_pos < 0.0f) out_pos += 1.0f;
        if (out_pos > 1.0f) out_pos -= 1.0f;
    }

    /* Output event */
    outlet_float(x->out_position, out_pos);
    outlet_bang(x->out_bang);

    /* Schedule next event */
    x->play_index++;
    if (x->play_index < x->num_events) {
        double next_pos = x->pattern[x->play_index];
        double delay = (next_pos - pos) * x->cycle_length_ms;
        if (delay < 0.1) delay = 0.1;
        clock_delay(x->playback_clock, delay);
    } else {
        outlet_float(x->out_count, (t_float)x->num_events);
    }
}

/* ------------------------------------------------------------------ */
/* Bang — record a tap or trigger playback                            */
/* ------------------------------------------------------------------ */

static void telomere_bang(t_telomere *x) {
    if (x->recording) {
        /* Record tap as fractional position in current cycle */
        double elapsed = clock_gettimesince(x->cycle_start_time);
        t_float pos = (t_float)(elapsed / x->cycle_length_ms);

        /* Clamp to 0–1 */
        while (pos >= 1.0f) pos -= 1.0f;
        if (pos < 0.0f) pos = 0.0f;

        /* Apply quantization */
        if (x->quantize_pct > 0.0f && x->grid > 0) {
            t_float step = 1.0f / (t_float)x->grid;
            t_float nearest = roundf(pos / step) * step;
            pos = pos + (nearest - pos) * x->quantize_pct;
            if (pos >= 1.0f) pos -= 1.0f;
        }

        pattern_append_event(x, pos);
        outlet_float(x->out_count, (t_float)x->num_events);
    } else {
        /* Trigger playback of current pattern */
        if (x->num_events == 0) return;
        x->play_index = 0;
        x->cycle_start_time = clock_getlogicaltime();
        /* Start at first event position */
        double delay = x->pattern[0] * x->cycle_length_ms;
        if (delay < 0.1) delay = 0.1;
        clock_delay(x->playback_clock, delay);
    }
}

/* ------------------------------------------------------------------ */
/* Float — set tempo                                                  */
/* ------------------------------------------------------------------ */

static void telomere_float(t_telomere *x, t_float f) {
    if (f > 0.0f) {
        x->tempo = f;
        x->cycle_length_ms = (60000.0 / (double)f) * x->beats_per_cycle;
    }
}

/* ------------------------------------------------------------------ */
/* Message dispatch — route to transform registry                     */
/* ------------------------------------------------------------------ */

static void telomere_anything(t_telomere *x, t_symbol *s, int argc, t_atom *argv) {
    t_transform_entry *entry = telomere_lookup_transform(s);

    if (!entry) {
        pd_error(x, "telomere: unknown message '%s'", s->s_name);
        return;
    }

    if (argc < entry->min_args ||
        (entry->max_args >= 0 && argc > entry->max_args)) {
        pd_error(x, "telomere: '%s' expects %d–%d args, got %d",
                 s->s_name, entry->min_args,
                 entry->max_args < 0 ? 999 : entry->max_args, argc);
        return;
    }

    entry->fn(x, argc, argv);
}

/* ------------------------------------------------------------------ */
/* Built-in messages (not routed through registry)                    */
/* ------------------------------------------------------------------ */

static void telomere_record(t_telomere *x, t_float f) {
    x->recording = (f != 0.0f);
    if (x->recording) {
        x->cycle_start_time = clock_getlogicaltime();
    }
    outlet_float(x->out_status, x->recording ? 1.0f : 0.0f);
}

static void telomere_clear(t_telomere *x) {
    pattern_clear(x);
    outlet_float(x->out_count, 0.0f);
}

static void telomere_quantize(t_telomere *x, t_float pct) {
    if (pct < 0.0f) pct = 0.0f;
    if (pct > 1.0f) pct = 1.0f;
    x->quantize_pct = pct;
}

static void telomere_grid(t_telomere *x, t_float g) {
    int gi = (int)g;
    if (gi < 1) gi = 1;
    if (gi > 128) gi = 128;
    x->grid = gi;
}

static void telomere_jitter(t_telomere *x, t_float amt) {
    if (amt < 0.0f) amt = 0.0f;
    if (amt > 1.0f) amt = 1.0f;
    x->jitter_amt = amt;
}

static void telomere_skip(t_telomere *x, t_float prob) {
    if (prob < 0.0f) prob = 0.0f;
    if (prob > 1.0f) prob = 1.0f;
    x->skip_prob = prob;
}

static void telomere_beats(t_telomere *x, t_float b) {
    int bi = (int)b;
    if (bi < 1) bi = 1;
    x->beats_per_cycle = bi;
    x->cycle_length_ms = (60000.0 / (double)x->tempo) * bi;
}

static void telomere_dump(t_telomere *x) {
    post("telomere: %d events, tempo=%.1f, grid=%d, q=%.2f",
         x->num_events, x->tempo, x->grid, x->quantize_pct);
    for (int i = 0; i < x->num_events; i++) {
        post("  [%d] %.6f", i, x->pattern[i]);
    }
}

static void telomere_help_msg(t_telomere *x) {
    post("telomere — available transforms:");
    t_transform_entry *cur = telomere_get_registry_head();
    while (cur) {
        post("  %-16s  args: %d-%d  %s",
             cur->name->s_name,
             cur->min_args,
             cur->max_args < 0 ? 999 : cur->max_args,
             cur->description);
        cur = cur->next;
    }
    post("---");
    post("  record <0|1>    start/stop recording");
    post("  clear           clear pattern");
    post("  quantize <0-1>  set quantize strength");
    post("  grid <n>        set grid subdivisions");
    post("  jitter <0-1>    set playback jitter");
    post("  skip <0-1>      set skip probability");
    post("  beats <n>       set beats per cycle");
    post("  dump            print pattern to console");
    (void)x;
}

/* ------------------------------------------------------------------ */
/* Constructor / Destructor                                           */
/* ------------------------------------------------------------------ */

static void *telomere_new(t_float tempo) {
    t_telomere *x = (t_telomere *)pd_new(telomere_class);

    /* Pattern storage */
    x->pattern_alloc = 32;
    x->pattern = (t_float *)getbytes(x->pattern_alloc * sizeof(t_float));
    x->num_events = 0;

    /* Euclidean */
    x->euclid_pattern = NULL;
    x->euclid_len = 0;

    /* Quantization */
    x->quantize_pct = 0.0f;
    x->grid = TELOMERE_DEFAULT_GRID;

    /* Clock */
    x->tempo = (tempo > 0.0f) ? tempo : TELOMERE_DEFAULT_TEMPO;
    x->beats_per_cycle = 4;
    x->cycle_length_ms = (60000.0 / (double)x->tempo) * x->beats_per_cycle;
    x->cycle_start_time = 0.0;

    /* Metric modulation */
    x->metric_num = 1.0f;
    x->metric_den = 1.0f;

    /* Playback */
    x->recording = 0;
    x->armed = 0;
    x->play_index = 0;

    /* Variation */
    x->jitter_amt = 0.0f;
    x->skip_prob = 0.0f;

    /* Outlets (right to left) */
    x->out_bang     = outlet_new(&x->x_obj, gensym("bang"));
    x->out_position = outlet_new(&x->x_obj, gensym("float"));
    x->out_count    = outlet_new(&x->x_obj, gensym("float"));
    x->out_status   = outlet_new(&x->x_obj, gensym("float"));

    /* Clock */
    x->playback_clock = clock_new(x, (t_method)telomere_tick);

    /* Inlet */
    x->f_inlet = 0.0f;

    return x;
}

static void telomere_free(t_telomere *x) {
    clock_free(x->playback_clock);
    if (x->pattern)
        freebytes(x->pattern, x->pattern_alloc * sizeof(t_float));
    if (x->euclid_pattern)
        freebytes(x->euclid_pattern, x->euclid_len * sizeof(int));
}

/* ------------------------------------------------------------------ */
/* Setup                                                              */
/* ------------------------------------------------------------------ */

EXTERN void telomere_setup(void) {
    telomere_class = class_new(
        gensym("telomere"),
        (t_newmethod)telomere_new,
        (t_freemethod)telomere_free,
        sizeof(t_telomere),
        CLASS_DEFAULT,
        A_DEFFLOAT, 0
    );

    class_addbang(telomere_class, (t_method)telomere_bang);
    class_addfloat(telomere_class, (t_method)telomere_float);
    class_addanything(telomere_class, (t_method)telomere_anything);

    /* Built-in messages */
    class_addmethod(telomere_class, (t_method)telomere_record,
                    gensym("record"), A_DEFFLOAT, 0);
    class_addmethod(telomere_class, (t_method)telomere_clear,
                    gensym("clear"), 0);
    class_addmethod(telomere_class, (t_method)telomere_quantize,
                    gensym("quantize"), A_DEFFLOAT, 0);
    class_addmethod(telomere_class, (t_method)telomere_grid,
                    gensym("grid"), A_DEFFLOAT, 0);
    class_addmethod(telomere_class, (t_method)telomere_jitter,
                    gensym("jitter"), A_DEFFLOAT, 0);
    class_addmethod(telomere_class, (t_method)telomere_skip,
                    gensym("skip"), A_DEFFLOAT, 0);
    class_addmethod(telomere_class, (t_method)telomere_beats,
                    gensym("beats"), A_DEFFLOAT, 0);
    class_addmethod(telomere_class, (t_method)telomere_dump,
                    gensym("dump"), 0);
    class_addmethod(telomere_class, (t_method)telomere_help_msg,
                    gensym("help"), 0);

    /* Register built-in transforms */
    telomere_transforms_builtins_setup();

    post("telomere: tap-pattern sequencer loaded");
}
