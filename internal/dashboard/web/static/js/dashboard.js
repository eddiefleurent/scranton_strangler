document.addEventListener('DOMContentLoaded', function() {
    console.log('Scranton Strangler Dashboard loaded');
    
    document.addEventListener('htmx:beforeRequest', function(event) {
        console.log('HTMX request starting:', event.detail.xhr.requestURL);
    });
    
    document.addEventListener('htmx:afterRequest', function(event) {
        if (event.detail.xhr.status === 200) {
            console.log('HTMX request completed successfully');
        } else {
            console.error('HTMX request failed:', event.detail.xhr.status);
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
            errorDiv.style.cssText = `
                position: fixed;
                top: 20px;
                right: 20px;
                background: #f85149;
                color: white;
                padding: 12px 16px;
                border-radius: 4px;
                font-size: 14px;
                z-index: 1000;
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
            `;
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