// ── AI Chat Module ───────────────────────────────────────────────────

import { apiPost } from './api.js';
import { showToast, escapeHtml } from './utils.js';

export function initAIChat() {
    const form = document.getElementById('ai-chat-form');
    const input = document.getElementById('ai-chat-input');
    
    form?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const question = input.value.trim();
        if (!question) return;

        // Append user message
        appendMessage(question, 'user');
        input.value = '';

        // Append temporary loading state message
        const loadingId = appendMessage('AI думает...', 'assistant loading');

        try {
            const resp = await apiPost('/ask-ai', { question });
            
            // Remove loading message
            removeMessage(loadingId);

            const answer = resp.answer || resp.response || 'Извините, не удалось получить внятный ответ.';
            appendMessage(answer, 'assistant');
        } catch (err) {
            removeMessage(loadingId);
            appendMessage(`Ошибка: ${err.message}. Попробуйте позже.`, 'assistant error');
        }
    });
}

let msgCounter = 0;

function appendMessage(text, senderClass) {
    const container = document.getElementById('ai-chat-messages');
    if (!container) return;

    msgCounter++;
    const msgId = `ai-msg-${msgCounter}`;
    
    const msgDiv = document.createElement('div');
    msgDiv.className = `ai-message ${senderClass}`;
    msgDiv.id = msgId;
    
    // Экранируем пользовательский/модельный текст ДО применения собственной
    // markdown-разметки чата — иначе символы `<`, `>`, `&` в тексте сломают
    // вёрстку, а разметка `<br>`/`<strong>`, добавляемая ниже, экранируется
    // вместе с ним (design.md, Decision 5).
    // Simplistic formatting for markdown paragraphs & lists
    const formattedText = escapeHtml(text)
        .replace(/\n/g, '<br>')
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
        
    msgDiv.innerHTML = formattedText;
    container.appendChild(msgDiv);

    // Scroll to bottom
    container.scrollTop = container.scrollHeight;
    
    return msgId;
}

function removeMessage(id) {
    const el = document.getElementById(id);
    if (el) el.remove();
}
