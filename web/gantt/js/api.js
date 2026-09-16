// ── API Client Module ────────────────────────────────────────────────

import { state } from './state.js';

const API_BASE = '/api/gantt';

function getHeaders() {
    const headers = {
        'Content-Type': 'application/json'
    };
    const token = localStorage.getItem('tg_sys_token');
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    return headers;
}

function handleHttpError(status, errData) {
    if (status === 401) {
        localStorage.removeItem('tg_sys_token');
        state.set('userProfile', null);
        
        // Show auth overlay
        document.getElementById('auth-overlay').classList.remove('hidden');
        document.getElementById('app').classList.add('hidden');

        // .status выставляется явно (в отличие от ветки ниже, где message —
        // технический sentinel, а не текст сервера), чтобы вызывающий код
        // (auth.js:checkAuth) мог отличить отказ авторизации от сетевой
        // ошибки по статусу, а не строковым сравнением (web-async-feedback).
        const error = new Error('UNAUTHORIZED');
        error.status = 401;
        throw error;
    }
    if (status === 403) {
        const appEl = document.getElementById('app');
        // Если #app ещё скрыт — это первичная проверка доступа (/profile),
        // пользователь вообще не зарегистрирован в системе: показываем полноэкранный оверлей.
        // Если #app уже виден — это точечная ошибка прав внутри уже открытого приложения
        // (например, голосование без назначенной роли), приложение трогать не нужно.
        if (appEl && appEl.classList.contains('hidden')) {
            // Show access denied overlay
            document.getElementById('auth-overlay').classList.add('hidden');
            appEl.classList.add('hidden');
            document.getElementById('denied-overlay').classList.remove('hidden');

            const error = new Error('FORBIDDEN');
            error.status = 403;
            throw error;
        }
        // #app уже показан — пробрасываем ошибку с реальным текстом дальше,
        // без изменения видимости приложения и оверлеев.
    }

    const message = errData?.error?.message || errData?.error || `HTTP error ${status}`;
    const code = errData?.error?.code || 'UNKNOWN_ERROR';
    
    const error = new Error(message);
    error.code = code;
    error.status = status;
    throw error;
}

export async function apiGet(path) {
    try {
        const resp = await fetch(`${API_BASE}${path}`, {
            method: 'GET',
            headers: getHeaders()
        });
        
        if (!resp.ok) {
            const errData = await resp.json().catch(() => ({}));
            handleHttpError(resp.status, errData);
        }
        
        return await resp.json();
    } catch (e) {
        if (e.message === 'UNAUTHORIZED' || e.message === 'FORBIDDEN') throw e;
        throw e;
    }
}

export async function apiPost(path, body) {
    try {
        const resp = await fetch(`${API_BASE}${path}`, {
            method: 'POST',
            headers: getHeaders(),
            body: JSON.stringify(body)
        });
        
        if (!resp.ok) {
            const errData = await resp.json().catch(() => ({}));
            handleHttpError(resp.status, errData);
        }
        
        return await resp.json();
    } catch (e) {
        if (e.message === 'UNAUTHORIZED' || e.message === 'FORBIDDEN') throw e;
        throw e;
    }
}

// apiPostMultipart — POST multipart/form-data (для отправки файлов, например
// FormData с картинкой диаграммы Ганта — export-gantt-chart-image, задача 6.1).
// apiGet/apiPost/apiPut/apiDelete шлют только JSON, поэтому обход
// авторизации/handleHttpError через отдельный fetch() без общего хелпера
// не годится (ux-brief.md раздел 4) — переиспользуем ту же авторизацию и
// централизованную обработку ошибок (401/403 и err.code), что и остальные
// методы. Content-Type НЕ выставляется вручную: для FormData браузер сам
// формирует заголовок "multipart/form-data; boundary=..." с уникальным
// boundary — свой Content-Type из getHeaders() (application/json) здесь не
// подходит и должен быть убран.
export async function apiPostMultipart(path, formData) {
    try {
        const headers = getHeaders();
        delete headers['Content-Type'];

        const resp = await fetch(`${API_BASE}${path}`, {
            method: 'POST',
            headers,
            body: formData
        });

        if (!resp.ok) {
            const errData = await resp.json().catch(() => ({}));
            handleHttpError(resp.status, errData);
        }

        return await resp.json();
    } catch (e) {
        if (e.message === 'UNAUTHORIZED' || e.message === 'FORBIDDEN') throw e;
        throw e;
    }
}

export async function apiPut(path, body) {
    try {
        const resp = await fetch(`${API_BASE}${path}`, {
            method: 'PUT',
            headers: getHeaders(),
            body: JSON.stringify(body)
        });
        
        if (!resp.ok) {
            const errData = await resp.json().catch(() => ({}));
            handleHttpError(resp.status, errData);
        }
        
        return await resp.json();
    } catch (e) {
        if (e.message === 'UNAUTHORIZED' || e.message === 'FORBIDDEN') throw e;
        throw e;
    }
}

export async function apiDelete(path, body) {
    try {
        const resp = await fetch(`${API_BASE}${path}`, {
            method: 'DELETE',
            headers: getHeaders(),
            body: body !== undefined ? JSON.stringify(body) : undefined
        });
        
        if (!resp.ok) {
            const errData = await resp.json().catch(() => ({}));
            handleHttpError(resp.status, errData);
        }
        
        return await resp.json();
    } catch (e) {
        if (e.message === 'UNAUTHORIZED' || e.message === 'FORBIDDEN') throw e;
        throw e;
    }
}
