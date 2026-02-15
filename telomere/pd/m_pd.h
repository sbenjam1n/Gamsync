/* ----------------------------------------------------------------
 * Minimal Pure Data header stub for standalone compilation.
 * Replace with the real m_pd.h from your Pd installation.
 * ---------------------------------------------------------------- */
#ifndef __m_pd_h_
#define __m_pd_h_

#include <stddef.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <math.h>

#ifdef _WIN32
#  define EXTERN __declspec(dllexport)
#else
#  define EXTERN __attribute__((visibility("default")))
#endif

typedef float  t_float;
typedef float  t_sample;
typedef int    t_int;

/* ---- atoms ---- */
#define A_FLOAT   0
#define A_SYMBOL  1
#define A_DEFFLOAT 0
#define A_DEFSYM   1

typedef struct _symbol {
    const char  *s_name;
    void       **s_thing;
    struct _symbol *s_next;
} t_symbol;

typedef union _word {
    t_float  w_float;
    t_symbol *w_symbol;
} t_word;

typedef struct _atom {
    int    a_type;
    t_word a_w;
} t_atom;

static inline t_float atom_getfloat(const t_atom *a) {
    return (a->a_type == A_FLOAT) ? a->a_w.w_float : 0.0f;
}

static inline t_float atom_getfloatarg(int which, int argc, const t_atom *argv) {
    if (which >= 0 && which < argc && argv[which].a_type == A_FLOAT)
        return argv[which].a_w.w_float;
    return 0.0f;
}

static inline t_int atom_getint(const t_atom *a) {
    return (t_int)atom_getfloat(a);
}

static inline t_int atom_getintarg(int which, int argc, const t_atom *argv) {
    return (t_int)atom_getfloatarg(which, argc, argv);
}

static inline t_symbol *atom_getsymbol(const t_atom *a) {
    return (a->a_type == A_SYMBOL) ? a->a_w.w_symbol : NULL;
}

static inline t_symbol *atom_getsymbolarg(int which, int argc, const t_atom *argv) {
    if (which >= 0 && which < argc && argv[which].a_type == A_SYMBOL)
        return argv[which].a_w.w_symbol;
    return NULL;
}

#define SETFLOAT(atom, f)  do { (atom)->a_type = A_FLOAT;  (atom)->a_w.w_float  = (f); } while(0)
#define SETSYMBOL(atom, s) do { (atom)->a_type = A_SYMBOL; (atom)->a_w.w_symbol = (s); } while(0)

/* ---- object system ---- */
typedef struct _text {
    void *dummy;
} t_object;

/* Define t_class as a concrete struct so sizeof works */
typedef struct _class {
    void *c_methods;
    void *c_name;
} t_class;

typedef void (*t_method)(void);
typedef void *(*t_newmethod)(void);
typedef void (*t_freemethod)(void *);

#define CLASS_DEFAULT 0

/* ---- gensym ---- */
#define SYMTAB_SIZE 1024
static t_symbol *_symtab[SYMTAB_SIZE];
static int _symtab_init = 0;

static inline unsigned _symhash(const char *s) {
    unsigned h = 5381;
    while (*s) h = ((h << 5) + h) + (unsigned char)*s++;
    return h % SYMTAB_SIZE;
}

static inline t_symbol *gensym(const char *s) {
    if (!_symtab_init) { memset(_symtab, 0, sizeof(_symtab)); _symtab_init = 1; }
    unsigned h = _symhash(s);
    t_symbol *sym = _symtab[h];
    while (sym) {
        if (strcmp(sym->s_name, s) == 0) return sym;
        sym = sym->s_next;
    }
    sym = (t_symbol *)malloc(sizeof(t_symbol));
    sym->s_name = strdup(s);
    sym->s_thing = NULL;
    sym->s_next = _symtab[h];
    _symtab[h] = sym;
    return sym;
}

/* ---- outlet/inlet stubs ---- */
typedef struct _outlet {
    struct _outlet *next;
    int type;
} t_outlet;

typedef struct _inlet {
    struct _inlet *next;
} t_inlet;

static inline t_outlet *outlet_new(t_object *owner, t_symbol *type) {
    t_outlet *o = (t_outlet *)calloc(1, sizeof(t_outlet));
    (void)owner; (void)type;
    return o;
}

static inline void outlet_bang(t_outlet *o) { (void)o; }
static inline void outlet_float(t_outlet *o, t_float f) { (void)o; (void)f; }
static inline void outlet_symbol(t_outlet *o, t_symbol *s) { (void)o; (void)s; }
static inline void outlet_list(t_outlet *o, t_symbol *s, int argc, t_atom *argv) {
    (void)o; (void)s; (void)argc; (void)argv;
}

static inline t_inlet *inlet_new(t_object *owner, void *dest, t_symbol *s1, t_symbol *s2) {
    (void)owner; (void)dest; (void)s1; (void)s2;
    return (t_inlet *)calloc(1, sizeof(t_inlet));
}

static inline t_inlet *floatinlet_new(t_object *owner, t_float *fp) {
    (void)owner; (void)fp;
    return (t_inlet *)calloc(1, sizeof(t_inlet));
}

/* ---- class system stubs ---- */
static inline t_class *class_new(t_symbol *name, t_newmethod newm,
    t_freemethod freem, size_t size, int flags, int arg1, ...)
{
    (void)name; (void)newm; (void)freem; (void)size; (void)flags; (void)arg1;
    return (t_class *)calloc(1, sizeof(t_class));
}

static inline void class_addbang(t_class *c, t_method fn) { (void)c; (void)fn; }
static inline void class_addfloat(t_class *c, t_method fn) { (void)c; (void)fn; }
static inline void class_addmethod(t_class *c, t_method fn, t_symbol *sel, int arg1, ...) {
    (void)c; (void)fn; (void)sel; (void)arg1;
}
static inline void class_addanything(t_class *c, t_method fn) { (void)c; (void)fn; }

/* ---- memory ---- */
static inline void *getbytes(size_t n) { return calloc(1, n); }
static inline void *resizebytes(void *p, size_t old, size_t new_sz) {
    (void)old; return realloc(p, new_sz);
}
static inline void freebytes(void *p, size_t n) { (void)n; free(p); }

/* ---- posting ---- */
#define post(...) do { fprintf(stderr, __VA_ARGS__); fprintf(stderr, "\n"); } while(0)
#define pd_error(obj, ...) do { (void)(obj); fprintf(stderr, "error: "); fprintf(stderr, __VA_ARGS__); fprintf(stderr, "\n"); } while(0)

/* ---- clock ---- */
typedef struct _clock {
    void *owner;
} t_clock;

static inline t_clock *clock_new(void *owner, t_method fn) {
    (void)fn;
    t_clock *c = (t_clock *)calloc(1, sizeof(t_clock));
    if (c) c->owner = owner;
    return c;
}
static inline void clock_delay(t_clock *c, double ms) { (void)c; (void)ms; }
static inline void clock_free(t_clock *c) { free(c); }
static inline double clock_getlogicaltime(void) { return 0.0; }
static inline double clock_gettimesince(double t) { (void)t; return 0.0; }
static inline double clock_getsystimeafter(double ms) { (void)ms; return 0.0; }

/* ---- pd_new ---- */
static inline void *pd_new(t_class *c) {
    (void)c;
    return calloc(1, 4096);
}

/* ---- canvas / sys ---- */
typedef void t_canvas;
static inline t_canvas *canvas_getcurrent(void) { return NULL; }
static inline const char *canvas_getdir(t_canvas *c) { (void)c; return "."; }

/* ---- symbol constants ---- */
#define s_bang     (*gensym("bang"))
#define s_list     (*gensym("list"))
#define s_float    (*gensym("float"))
#define s_anything (*gensym("anything"))

#endif /* __m_pd_h_ */
