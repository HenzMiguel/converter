document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('convertForm');
  const fileInput = document.getElementById('file');
  const fileName = document.getElementById('fileName');

  const fromSelect = document.getElementById('from');
  const toSelect = document.getElementById('format');
  const fromTitle = document.getElementById('fromTitle');
  const toTitle = document.getElementById('toTitle');
  const fromDescription = document.getElementById('fromDescription');
  const toDescription = document.getElementById('toDescription');
  const fromLink = document.getElementById('fromLink');
  const toLink = document.getElementById('toLink');

  const submitButton = document.getElementById('submitButton');
  const loadingModal = document.getElementById('loadingModal');
  const resultModal = document.getElementById('resultModal');
  const resultTitle = document.getElementById('resultTitle');
  const resultMessage = document.getElementById('resultMessage');
  const downloadButton = document.getElementById('downloadButton');

  const formatSearch = document.getElementById('formatSearch');

  const formatAliases = {
    jpg: ['jpg', 'jpeg'],
    jpeg: ['jpeg', 'jpg'],
    tif: ['tif', 'tiff'],
    tiff: ['tiff', 'tif'],
    m4v: ['m4v', 'mp4'],
  };

  const cache = new Map();

  async function loadInfo(format) {
    if (cache.has(format)) {
      return cache.get(format);
    }
    const res = await fetch(`/formatinfo?format=${encodeURIComponent(format)}`);
    const data = res.ok ? await res.json() : { extract: 'Descrição indisponível.', url: '#' };
    cache.set(format, data);
    return data;
  }

  async function updateSide(select, title, description, link) {
    const format = select.value;
    title.textContent = format;
    description.textContent = 'Carregando descrição...';
    const info = await loadInfo(format);
    description.textContent = info.extract || 'Descrição indisponível.';
    link.href = info.url || '#';
  }

  // Garante que a conversão nunca ocorra de um formato para ele mesmo (ex.: avi -> avi).
  function avoidSameFormat(changedSelect, otherSelect) {
    if (otherSelect.value !== changedSelect.value) {
      return false;
    }
    const alternative = Array.from(otherSelect.options).find(o => o.value !== changedSelect.value);
    if (!alternative) {
      return false;
    }
    otherSelect.value = alternative.value;
    return true;
  }

  // Restringe o select de saída à categoria do arquivo enviado (imagem ou vídeo)
  // e aplica o filtro de busca digitado, escondendo as opções que não combinam.
  let activeCategory = null;

  function applyFormatFilters() {
    const term = formatSearch.value.trim().toLowerCase();
    const visibleValues = [];

    Array.from(toSelect.options).forEach((option) => {
      const matchesCategory = !activeCategory || option.dataset.category === activeCategory;
      const matchesSearch = !term || option.value.toLowerCase().includes(term);
      const visible = matchesCategory && matchesSearch;
      option.hidden = !visible;
      if (visible) visibleValues.push(option.value);
    });

    if (visibleValues.length > 0 && !visibleValues.includes(toSelect.value)) {
      toSelect.value = visibleValues[0];
      toSelect.dispatchEvent(new Event('change', { bubbles: true }));
    }
  }

  formatSearch.addEventListener('input', applyFormatFilters);

  fromSelect.addEventListener('change', () => {
    updateSide(fromSelect, fromTitle, fromDescription, fromLink);
    if (avoidSameFormat(fromSelect, toSelect)) {
      updateSide(toSelect, toTitle, toDescription, toLink);
    }
  });

  toSelect.addEventListener('change', () => {
    updateSide(toSelect, toTitle, toDescription, toLink);
    if (avoidSameFormat(toSelect, fromSelect)) {
      updateSide(fromSelect, fromTitle, fromDescription, fromLink);
    }
  });

  fileInput.addEventListener('change', () => {
    const file = fileInput.files[0];
    fileName.textContent = file ? file.name : 'Nenhum arquivo selecionado';
    if (!file) {
      activeCategory = null;
      applyFormatFilters();
      return;
    }

    const extension = file.name.includes('.') ? file.name.split('.').pop().toLowerCase() : '';
    const candidates = formatAliases[extension] ?? [extension];
    const option = Array.from(fromSelect.options).find(o => candidates.includes(o.value.toLowerCase()));
    if (option) {
      fromSelect.value = option.value;
      fromSelect.dispatchEvent(new Event('change', { bubbles: true }));
      activeCategory = option.dataset.category || null;
    } else {
      activeCategory = null;
    }
    applyFormatFilters();
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    submitButton.disabled = true;
    loadingModal.showModal();

    let result;
    try {
      const res = await fetch(form.action, {
        method: 'POST',
        body: new FormData(form),
        headers: { Accept: 'application/json' },
      });
      result = await res.json();
      result.ok = res.ok;
    } catch {
      result = { ok: false, error: 'Falha de comunicação com o servidor.' };
    }

    loadingModal.close();
    submitButton.disabled = false;

    if (result.ok && result.downloadName) {
      resultTitle.textContent = 'Arquivo pronto';
      resultMessage.textContent = 'A conversão foi concluída com sucesso.';
      downloadButton.href = `/download/${result.downloadName}`;
      downloadButton.classList.remove('hidden');
    } else {
      resultTitle.textContent = 'Não foi possível converter';
      resultMessage.textContent = result.error || 'Ocorreu um erro inesperado.';
      downloadButton.classList.add('hidden');
    }
    resultModal.showModal();
  });

  updateSide(fromSelect, fromTitle, fromDescription, fromLink);
  updateSide(toSelect, toTitle, toDescription, toLink);
});
