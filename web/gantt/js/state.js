// ── State Management Module ──────────────────────────────────────────

class State {
    constructor() {
        this._state = {
            userProfile: null,       // { id, first_name, telegram_id, role }
            teams: [],               // Array of Team
            selectedTeamId: '',      // Current team UUID
            epics: [],               // Array of Epic
            selectedEpicId: '',      // Current epic UUID for Gantt / Scoring
            tasks: [],               // Array of GanttTask
            viewMode: 'Day',         // Day, Week, Month
            activeTab: 'gantt',      // gantt, scoring, admin, ai-chat
            roles: []                // Array of Roles loaded from backend
        };
        this._listeners = {};
    }

    get(key) {
        return this._state[key];
    }

    set(key, value) {
        const oldValue = this._state[key];
        if (JSON.stringify(oldValue) === JSON.stringify(value)) return;
        this._state[key] = value;
        this._notify(key, value, oldValue);
    }

    // Subscribe to state changes
    subscribe(key, callback) {
        if (!this._listeners[key]) {
            this._listeners[key] = [];
        }
        this._listeners[key].push(callback);
        // Call immediately with current value if exists
        if (this._state[key] !== undefined) {
            callback(this._state[key], undefined);
        }
        return () => {
            this._listeners[key] = this._listeners[key].filter(cb => cb !== callback);
        };
    }

    _notify(key, newValue, oldValue) {
        if (this._listeners[key]) {
            this._listeners[key].forEach(callback => {
                try {
                    callback(newValue, oldValue);
                } catch (e) {
                    console.error(`Error in state listener for key "${key}":`, e);
                }
            });
        }
    }
}

export const state = new State();
