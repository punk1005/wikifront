class WIKICONST {
    get APP_PREFIX() {
        return '/wiki';
    } 


}
const WIC = new WIKICONST();


// Простая функция экранирования строк от XSS
function escapeHtml(text) {
    if (!text) return '';
        return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
    }