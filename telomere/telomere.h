/* telomere.h — Main struct definition for the telomere Pd external */
#ifndef TELOMERE_H
#define TELOMERE_H

#include "m_pd.h"

#define TELOMERE_MAX_EVENTS   256
#define TELOMERE_DEFAULT_GRID 16
#define TELOMERE_DEFAULT_TEMPO 120.0f

typedef struct _telomere {
    t_object  x_obj;

    /* --- Pattern storage --- */
    t_float  *pattern;          /* dynamically allocated event buffer      */
    int       num_events;       /* current number of recorded events       */
    int       pattern_alloc;    /* allocated capacity                      */

    /* --- Euclidean pattern --- */
    int      *euclid_pattern;   /* boolean hits for euclidean rhythm       */
    int       euclid_len;       /* length of euclidean pattern             */

    /* --- Quantization --- */
    t_float   quantize_pct;     /* 0.0 = free, 1.0 = fully quantized      */
    int       grid;             /* grid subdivisions per cycle             */

    /* --- Clock / timing --- */
    t_float   tempo;            /* BPM                                     */
    double    cycle_length_ms;  /* derived: (60000 / tempo) * beats/cycle  */
    double    cycle_start_time; /* logical time of current cycle start     */
    int       beats_per_cycle;  /* how many beats form one cycle           */

    /* --- Metric modulation --- */
    t_float   metric_num;       /* numerator for metric modulation ratio   */
    t_float   metric_den;       /* denominator for metric modulation ratio */

    /* --- Playback / recording --- */
    int       recording;        /* 1 = recording taps, 0 = idle           */
    int       armed;            /* 1 = armed for next cycle                */

    /* --- Variation parameters --- */
    t_float   jitter_amt;       /* random displacement amount (0.0–1.0)    */
    t_float   skip_prob;        /* probability of skipping an event        */

    /* --- Outlets --- */
    t_outlet *out_bang;         /* fires a bang per event during playback  */
    t_outlet *out_position;     /* outputs event position as float         */
    t_outlet *out_count;        /* outputs current event count             */
    t_outlet *out_status;       /* outputs status messages                 */

    /* --- Clock object --- */
    t_clock  *playback_clock;   /* clock for scheduling playback events    */
    int       play_index;       /* current index during playback           */

    /* --- Inlet value --- */
    t_float   f_inlet;          /* dummy float inlet                       */
} t_telomere;

/* Externally visible class pointer */
extern t_class *telomere_class;

/* Setup function */
EXTERN void telomere_setup(void);

#endif /* TELOMERE_H */
