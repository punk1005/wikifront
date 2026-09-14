// Функция мягкого удаления (для писателей и модераторов)
async function markForDeletion(slug) {
    if (!confirm("Вы уверены, что хотите пометить эту статью на удаление?")) return;

    try {
        const response = await fetch(`/api/wiki/${slug}/pending-delete`, { method: 'POST' });
        if (response.ok) {
            alert("Статья успешно отправлена на рассмотрение админу.");
            window.location.href = "/folders";
        } else {
            alert("Ошибка при попытке пометить статью.");
        }
    } catch (err) {
        console.error(err);
    }
}

// Функция жесткого удаления (Строго для Админа)
async function hardDeleteArticle(slug) {
    if (!confirm("ВНИМАНИЕ! Статья, все её блоки и прикрепленные файлы будут стерты навсегда. Продолжить?")) return;

    try {
        const response = await fetch(`/api/wiki/${slug}/hard-delete`, { method: 'POST' });
        if (response.ok) {
            // Удаляем строку из таблицы на фронте без перезагрузки страницы
            const row = document.getElementById(`row-${slug}`);
            if (row) row.remove();
            alert("Статья физически удалена из системы.");
        } else {
            alert("Не удалось выполнить удаление. Возможно, недостаточно прав.");
        }
    } catch (err) {
        console.error(err);
    }
}
