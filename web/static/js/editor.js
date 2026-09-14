// Хранилище для инстансов редакторов, чтобы вытаскивать из них HTML перед сохранением
let editorsList = [];

// Инициализация: если блоки уже были, создаем их, если нет — создаем один пустой блок
function initEditor(initialBlocks) {
    if (initialBlocks && initialBlocks.length > 0) {
        initialBlocks.forEach(block => {
            addNewBlock(block.content);
        });
    } else {
        addNewBlock(''); // Один стартовый пустой блок для новой статьи
    }

    // Вешаем событие на отправку формы
    document.getElementById('articleForm').addEventListener('submit', handleFormSubmit);
}

// Функция динамического добавления блока в DOM-дерево
function addNewBlock(htmlContent = '') {
    const container = document.getElementById('blocksContainer');
    const blockIndex = editorsList.length;

    // Создаем обертку для блока с кнопкой удаления внутри статьи
    const blockWrapper = document.createElement('div');
    blockWrapper.className = 'card mb-3 border border-secondary-subtle block-item';
    blockWrapper.setAttribute('data-index', blockIndex);

    blockWrapper.innerHTML = `
        <div class="card-header bg-light d-flex justify-content-between align-items-center py-1">
            <span class="text-muted small fw-bold">Блок #${blockIndex + 1}</span>
            <button type="button" class="btn btn-link text-danger btn-sm p-0 text-decoration-none" onclick="removeBlock(this)">Удалить блок</button>
        </div>
        <div class="card-body p-0">
            <!-- Контейнер, куда Quill вошьет свой редактор -->
            <div class="editor-container" style="height: 150px; border: none;">${htmlContent}</div>
        </div>
    `;

    container.appendChild(blockWrapper);

    // Инициализируем Quill на созданном контейнере
    const editorElement = blockWrapper.querySelector('.editor-container');
    const quill = new Quill(editorElement, {
        theme: 'snow',
        modules: {
            toolbar: [
                ['bold', 'italic', 'underline'],
                [{ 'list': 'ordered'}, { 'list': 'bullet' }],
                ['clean']
            ]
        }
    });

    // Сохраняем ссылку на объект редактора
    editorsList.push(quill);
}

// Удаление конкретного блока
function removeBlock(button) {
    const blockItem = button.closest('.block-item');
    const indexToRemove = parseInt(blockItem.getAttribute('data-index'));
    
    blockItem.remove();
    // Удаляем из массива инстансов
    editorsList.splice(indexToRemove, 1);
    
    // Пересчитываем заголовки блоков (Блок #1, Блок #2...), чтобы пользователь не путался
    const allRemaining = document.querySelectorAll('.block-item');
    allRemaining.forEach((item, idx) => {
        item.setAttribute('data-index', idx);
        item.querySelector('.card-header span').innerText = `Блок #${idx + 1}`;
    });
}

// Сборка данных и отправка по API на wikifront -> который проксирует в wikiapi
async function handleFormSubmit(e) {
    e.preventDefault();

    // Собираем HTML со всех активных блоков по порядку
    const blocksData = editorsList.map((quill, index) => {
        return {
            content: quill.root.innerHTML, // Чистый HTML из редактора
            ordinal_position: index
        };
    });

    const payload = {
        title: document.getElementById('articleTitle').value,
        folder_id: parseInt(document.getElementById('folderSelect').value),
        blocks: blocksData
    };

    const isEdit = document.getElementById('isEditMode').value === 'true';
    const slug = document.getElementById('articleSlug').value;
    
    // Определяем URL: либо создание, либо обновление существующей статьи
    const url = isEdit ? `/api/wiki/${slug}` : '/api/wiki/create';

    try {
        const response = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        if (response.ok) {
            const result = await response.json();
            window.location.href = `/wiki/${result.slug}`; // Редирект на созданную/измененную статью
        } else {
            alert('Что-то пошло не так при сохранении статьи');
        }
    } catch (err) {
        console.error('Ошибка сети:', err);
    }
}
