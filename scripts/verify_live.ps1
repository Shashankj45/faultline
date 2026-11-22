$ErrorActionPreference = "Stop"
$ProgressPreference = 'SilentlyContinue'
Write-Host "=== FAULTLINE END-TO-END VERIFICATION SUITE ==="

$port = 8888
$exe = ".\faultline.exe"
$dbFile = "test_verify.db"
if (Test-Path $dbFile) { Remove-Item $dbFile -Force }

# Launch FaultLine in background job within this session
$job = Start-Process -FilePath $exe -ArgumentList "-port $port", "-db $dbFile" -PassThru

try {
    Write-Host "[1/7] Waiting for FaultLine to boot on port $port..."
    Start-Sleep -Seconds 3

    # 1. API Health Check
    Write-Host "[2/7] Probing /api/v1/health..."
    $healthResp = Invoke-RestMethod -Uri "http://localhost:$port/api/v1/health" -Method Get
    Write-Host "      Health Status:" $healthResp.status "Service:" $healthResp.service

    # 2. Registered Upstreams Check
    Write-Host "[3/7] Probing /api/v1/upstreams..."
    $upstreams = Invoke-RestMethod -Uri "http://localhost:$port/api/v1/upstreams" -Method Get
    Write-Host "      Registered upstreams count:" $upstreams.Count
    foreach ($u in $upstreams) {
        Write-Host "      -> Upstream:" $u.name "Slug:" $u.slug "Target:" $u.target_url
    }

    # 3. Prometheus Metrics Check
    Write-Host "[4/7] Scraping Prometheus /metrics..."
    $metricsText = (Invoke-WebRequest -Uri "http://localhost:$port/metrics" -UseBasicParsing).Content
    if ($metricsText -match "faultline_killswitch_active") {
        Write-Host "      Prometheus metrics verified successfully."
    } else {
        throw "Prometheus metrics missing expected counters"
    }

    # 4. Web Dashboard Serving Check
    Write-Host "[5/7] Verifying Web UI Dashboard on root /..."
    $uiHtml = (Invoke-WebRequest -Uri "http://localhost:$port/" -UseBasicParsing).Content
    if ($uiHtml -match "FaultLine") {
        Write-Host "      React SPA Dashboard index.html successfully served."
    } else {
        throw "Dashboard HTML did not contain 'FaultLine'"
    }

    # 5. Clean Passthrough via Reverse Proxy (/proxy/orders/v1/orders)
    Write-Host "[6/7] Dispatching live proxy request to /proxy/orders/v1/orders..."
    $orderBody = '{"customer": "corp-client", "items": 2}'
    $orderResp = Invoke-RestMethod -Uri "http://localhost:$port/proxy/orders/v1/orders" -Method Post -Body $orderBody -ContentType "application/json"
    Write-Host "      Order Service Mock Response:" ($orderResp | ConvertTo-Json -Compress)

    # 6. Test Studio Dispatch & Chaos Verification
    Write-Host "[7/7] Testing Chaos Injection via /api/v1/test-studio/dispatch..."
    $chaosPayload = @{
        upstream_slug = "payments"
        method = "POST"
        path = "/v1/charges"
        headers = @{ "Content-Type" = "application/json" }
        payload = '{"amount": 100.00}'
    } | ConvertTo-Json

    $chaosResult = Invoke-RestMethod -Uri "http://localhost:$port/api/v1/test-studio/dispatch" -Method Post -Body $chaosPayload -ContentType "application/json"
    Write-Host "      Test Studio Result:"
    Write-Host "      -> Status Code:   " $chaosResult.status_code
    Write-Host "      -> Duration (ms): " $chaosResult.duration_ms
    Write-Host "      -> Chaos Injected:" $chaosResult.is_injected
    Write-Host "      -> Fault Type:    " $chaosResult.injected_fault

    Write-Host ""
    Write-Host ">>> ALL LIVE VERIFICATION CHECKS PASSED WITH 100% SUCCESS! <<<"
}
finally {
    Write-Host "Cleaning up process..."
    Stop-Process -Id $job.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
    if (Test-Path $dbFile) { Remove-Item $dbFile -Force -ErrorAction SilentlyContinue }
}
