document.addEventListener('DOMContentLoaded', function() {
    console.log('Scranton Strangler Dashboard loaded');
    
    document.addEventListener('htmx:beforeRequest', function(event) {
        const url = event.detail.requestConfig?.path || 
                   event.detail.requestConfig?.url || 
                   event.detail.xhr?.responseURL || 
                   'unknown';
        console.log('HTMX request starting:', url);
    });
    
    document.addEventListener('htmx:afterRequest', function(event) {
        if (event.detail && event.detail.successful === true) {
            console.log('HTMX request completed successfully');
        } else {
            const status = event.detail && event.detail.xhr && event.detail.xhr.status;
            console.error('HTMX request failed:', status);
        }
    });
    
    document.addEventListener('htmx:responseError', function(event) {
        console.error('HTMX response error:', event.detail);
        showError('Failed to update data. Check network connection.');
    });
    
    document.addEventListener('htmx:timeout', function(event) {
        console.error('HTMX timeout:', event.detail);
        showError('Request timed out. Retrying...');
    });
    
    let errorTimeout;
    
    function showError(message) {
        let errorDiv = document.getElementById('error-message');
        if (!errorDiv) {
            errorDiv = document.createElement('div');
            errorDiv.id = 'error-message';
            errorDiv.className = 'error-banner';
            document.body.appendChild(errorDiv);
        }
        
        errorDiv.textContent = message;
        errorDiv.style.display = 'block';
        
        clearTimeout(errorTimeout);
        errorTimeout = setTimeout(() => {
            errorDiv.style.display = 'none';
        }, 5000);
    }
});

function showPositionDetail() {
    const detailSection = document.getElementById('position-detail-section');
    if (detailSection) {
        detailSection.classList.remove('hidden');
        detailSection.scrollIntoView({ behavior: 'smooth' });
    }
}

function hidePositionDetail() {
    const detailSection = document.getElementById('position-detail-section');
    if (detailSection) {
        detailSection.classList.add('hidden');
    }
}

document.addEventListener('click', function(e) {
    if (e.target.dataset.action === 'close-detail') {
        hidePositionDetail();
    }
});