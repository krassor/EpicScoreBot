// ── Utilities Module ─────────────────────────────────────────────────

export function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    if (!container) return;
    
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    
    // Add appropriate icon prefix
    let icon = 'ℹ️ ';
    if (type === 'success') icon = '✅ ';
    if (type === 'error') icon = '❌ ';
    
    toast.textContent = icon + message;
    container.appendChild(toast);
    
    // Animate and remove
    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transform = 'translateX(20px)';
        setTimeout(() => toast.remove(), 250);
    }, 4000);
}

// Карта известных серверных сообщений об ошибках на понятный пользователю русский текст
const KNOWN_ERROR_MESSAGES = {
    'user has no role assigned': 'Вам не назначена роль в системе, поэтому вы не можете голосовать. Обратитесь к администратору.',
    'user not registered in system': 'Вы не зарегистрированы в системе как участник. Обратитесь к администратору.',
    'user not registered': 'Вы не зарегистрированы в системе как участник. Обратитесь к администратору.',
    'forbidden': 'Недостаточно прав для этого действия.'
};

export function showErrorModal(message) {
    const text = KNOWN_ERROR_MESSAGES[message] || message;

    let modal = document.getElementById('modal-error');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'modal-error';
        modal.className = 'modal hidden';
        document.body.appendChild(modal);
    }

    modal.innerHTML = `
        <div class="modal-overlay"></div>
        <div class="modal-content" style="max-width: 420px;">
            <div class="modal-header">
                <h2>Не удалось выполнить действие</h2>
            </div>
            <div class="modal-body">
                <p>${text}</p>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-primary" id="modal-error-close">Понятно</button>
            </div>
        </div>
    `;

    modal.classList.remove('hidden');

    const close = () => modal.classList.add('hidden');
    modal.querySelector('#modal-error-close').onclick = close;
    modal.querySelector('.modal-overlay').onclick = close;
}

export function formatDate(date) {
    if (!date) return '';
    const d = new Date(date);
    if (isNaN(d.getTime())) return '';
    const year = d.getFullYear();
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}
