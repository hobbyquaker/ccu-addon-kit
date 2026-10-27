/*
 * A stand-in for the CCU's tclrega.so, for the kit's tests (MIT).
 *
 * The kit's only ReGa call is the session check
 *   Write(system.GetSessionVarStr('<sid>'));
 * whose second result element is the session's data for a live session and "" for an
 * expired or unknown one. This stub answers exactly that from a file, so a test makes a
 * session live, expired or of another shape by writing one line:
 *
 *   $KIT_REGA_SIDS (default /tmp/kit-rega-sids), one session per line:
 *     <sid> <answer>
 *   e.g.  Adm1nSess1 1001;8;Admin;;;     the CCU3/OpenCCU shape: uid;upl;user;first;last;
 *         LiteAlias1 anna                the openccu-lite shim's shape: the user name only
 *
 * Every script it is given is appended to $KIT_REGA_LOG when that is set, so a test can see
 * what went into ReGa. Any other script raises a Tcl error, as openccu-lite's shim does.
 *
 * Builds against Tcl 8.2 (char *argv[]) and 8.6 (const char *argv[]).
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <tcl.h>

#ifndef CONST84
#define CONST84
#endif

#define MARKER "Write(system.GetSessionVarStr('"
#define TAIL "'));"

static const char *env_or(const char *name, const char *fallback) {
    const char *v = getenv(name);
    return (v != NULL && v[0] != '\0') ? v : fallback;
}

static void lookup(const char *sid, char *answer, size_t size) {
    char line[512];
    size_t n = strlen(sid);
    FILE *file;

    answer[0] = '\0';
    if (n == 0) {
        return;
    }
    file = fopen(env_or("KIT_REGA_SIDS", "/tmp/kit-rega-sids"), "r");
    if (file == NULL) {
        return;
    }
    while (fgets(line, sizeof(line), file) != NULL) {
        char *nl = strchr(line, '\n');
        if (nl != NULL) {
            *nl = '\0';
        }
        if (strncmp(line, sid, n) == 0 && line[n] == ' ') {
            strncpy(answer, line + n + 1, size - 1);
            answer[size - 1] = '\0';
            break;
        }
    }
    fclose(file);
}

static int rega_script_cmd(ClientData data, Tcl_Interp *interp, int argc, CONST84 char *argv[]) {
    const char *script = argc > 1 ? argv[1] : "";
    const char *log = getenv("KIT_REGA_LOG");
    size_t ml = strlen(MARKER), tl = strlen(TAIL), sl = strlen(script);
    char sid[64];
    char answer[256];
    Tcl_Obj *result;

    (void)data;
    if (log != NULL && log[0] != '\0') {
        FILE *f = fopen(log, "a");
        if (f != NULL) {
            fprintf(f, "%s\n", script);
            fclose(f);
        }
    }
    if (sl < ml + tl || strncmp(script, MARKER, ml) != 0 || strcmp(script + sl - tl, TAIL) != 0
            || sl - ml - tl >= sizeof(sid)) {
        Tcl_SetResult(interp, (char *)"stub tclrega: only the session check is answered", TCL_STATIC);
        return TCL_ERROR;
    }
    memcpy(sid, script + ml, sl - ml - tl);
    sid[sl - ml - tl] = '\0';
    lookup(sid, answer, sizeof(answer));

    result = Tcl_NewListObj(0, NULL);
    /* the real rega_script returns {<stdout> <result>} */
    Tcl_ListObjAppendElement(interp, result, Tcl_NewStringObj("", -1));
    Tcl_ListObjAppendElement(interp, result, Tcl_NewStringObj(answer, -1));
    Tcl_SetObjResult(interp, result);
    return TCL_OK;
}

/* `load tclrega.so` derives this name from the file name. */
int Tclrega_Init(Tcl_Interp *interp) {
    Tcl_CreateCommand(interp, "rega_script", (Tcl_CmdProc *)rega_script_cmd, NULL, NULL);
    return TCL_OK;
}
