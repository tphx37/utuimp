#ifndef UTUIMP_LOCAL_SINK_WINDOWS_H
#define UTUIMP_LOCAL_SINK_WINDOWS_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* utuimp_localsink_t;

typedef void (*utuimp_localsink_eos_cb_t)(uintptr_t user_data, uint64_t gen);
typedef void (*utuimp_localsink_err_cb_t)(uintptr_t user_data, uint64_t gen, char* err_msg);

utuimp_localsink_t utuimp_localsink_create(uintptr_t user_data, utuimp_localsink_eos_cb_t on_eos, utuimp_localsink_err_cb_t on_err);
// utuimp_localsink_load opens path_utf8, resolves its duration (out_duration_sec),
// and starts playback when autoplay != 0. On failure it writes an error message
// into err_buf and returns non-zero.
int utuimp_localsink_load(utuimp_localsink_t handle, const char* path_utf8, uint64_t gen, int autoplay, double* out_duration_sec, char* err_buf, int err_buf_len);
void utuimp_localsink_play(utuimp_localsink_t handle);
void utuimp_localsink_pause(utuimp_localsink_t handle);
void utuimp_localsink_stop(utuimp_localsink_t handle);
void utuimp_localsink_seek(utuimp_localsink_t handle, double seconds);
void utuimp_localsink_set_volume(utuimp_localsink_t handle, double volume);
void utuimp_localsink_set_eq(utuimp_localsink_t handle, const double* gains_db, int count);
void utuimp_localsink_unload(utuimp_localsink_t handle);
double utuimp_localsink_position(utuimp_localsink_t handle);
double utuimp_localsink_duration(utuimp_localsink_t handle);
int utuimp_localsink_is_playing(utuimp_localsink_t handle);
int utuimp_localsink_read_tap(utuimp_localsink_t handle, float* dst, int n, double* ahead, int64_t* end);
void utuimp_localsink_destroy(utuimp_localsink_t handle);

#ifdef __cplusplus
}
#endif

#endif // UTUIMP_LOCAL_SINK_WINDOWS_H
