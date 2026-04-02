(function () {
  function initNetworkCharts() {
    document.querySelectorAll('#network-chart').forEach(function (node) {
      if (node.dataset.chartBound === 'true') return;
      node.dataset.chartBound = 'true';
      var chart = echarts.init(node, null, { renderer: 'canvas' });
      var currentURL = node.dataset.chartUrl;
      var refresh = parseDuration(node.dataset.chartRefresh || '20s');

      function render(url) {
        currentURL = url || currentURL;
        fetch(currentURL, { headers: { 'X-Requested-With': 'fetch' } })
          .then(function (r) { return r.json(); })
          .then(function (payload) {
            chart.setOption({
              backgroundColor: 'transparent',
              animation: false,
              tooltip: { trigger: 'axis' },
              grid: { left: 20, right: 20, top: 20, bottom: 30, containLabel: true },
              legend: { data: payload.series.map(function (item) { return item.name; }), textStyle: { color: '#bec8d2' } },
              xAxis: {
                type: 'time',
                boundaryGap: false,
                axisLine: { lineStyle: { color: 'rgba(190,200,210,0.2)' } },
                axisLabel: { color: '#bec8d2' }
              },
              yAxis: {
                type: 'value',
                axisLine: { show: false },
                splitLine: { lineStyle: { color: 'rgba(190,200,210,0.12)' } },
                axisLabel: {
                  color: '#bec8d2',
                  formatter: function (value) { return formatRate(value); }
                }
              },
              series: payload.series.map(function (item, idx) {
                return {
                  name: item.name,
                  type: 'line',
                  smooth: true,
                  symbol: 'none',
                  lineStyle: { width: 3, color: idx === 0 ? '#89ceff' : '#ffb86e' },
                  areaStyle: { opacity: idx === 0 ? 0.15 : 0.08 },
                  data: item.points.map(function (point) { return [point.timestamp, point.value]; })
                };
              })
            });
          })
          .catch(function () {});
      }

      render(currentURL);
      if (refresh > 0) {
        window.setInterval(function () { render(currentURL); }, refresh);
      }

      document.querySelectorAll('[data-chart-range]').forEach(function (button) {
        button.addEventListener('click', function () {
          var range = button.getAttribute('data-chart-range');
          var url = currentURL.replace(/range=[^&]+/, 'range=' + range);
          render(url);
        });
      });

      window.addEventListener('resize', function () { chart.resize(); });
    });
  }

  function parseDuration(value) {
    var match = /^([0-9]+)(ms|s|m)$/.exec(value);
    if (!match) return 0;
    var amount = parseInt(match[1], 10);
    if (match[2] === 'ms') return amount;
    if (match[2] === 'm') return amount * 60 * 1000;
    return amount * 1000;
  }

  function formatRate(value) {
    var units = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s'];
    var idx = 0;
    var num = value;
    while (num >= 1024 && idx < units.length - 1) {
      num /= 1024;
      idx++;
    }
    return num.toFixed(idx === 0 ? 0 : 1) + ' ' + units[idx];
  }

  document.addEventListener('DOMContentLoaded', initNetworkCharts);
  document.body.addEventListener('htmx:afterSwap', initNetworkCharts);
})();
