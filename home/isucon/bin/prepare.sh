#!/bin/sh
set -ex
################################################################################
echo "# Prepare"
################################################################################

cat > /tmp/prepared_env <<EOF
prepared_time="`date +'%Y-%m-%d %H:%M:%S'`"
nginx_access_log="/var/log/nginx/access.log"
#nginx_access_json="/var/log/nginx/access.json"
nginx_error_log="/var/log/nginx/error.log"
#mysql_slow_log="/var/log/mysql/slow.log"
#mysql_error_log="/var/log/mysql/error.log"
result_dir="/result"
data_dir="\${result_dir}/data"
EOF

. /tmp/prepared_env

# app
(
  cd ${HOME}/webapp/go
  make
)
sudo systemctl restart isureal-go

# preset画像が欠けていると画像配信が壊れるため、最低1枚存在することを確認する。
if [ -z "$(find /home/isucon/webapp/preset-images -type f -print -quit 2>/dev/null)" ]; then
  echo "preset images are missing" >&2
  exit 1
fi

# 全台の実行時画像キャッシュを空にする。presetは削除しない。
rm -rf /home/isucon/webapp/cache/images/*

for s in america1 europe1 ; do
  rsync -a --delete /home/isucon/webapp/go/ $s:/home/isucon/webapp/go/
  cat /home/isucon/isureal.conf | ssh $s sudo tee /etc/nginx/sites-available/isureal.conf >/dev/null
  if ! ssh $s "test -n \"\$(find /home/isucon/webapp/preset-images -type f -print -quit 2>/dev/null)\""; then
    echo "preset images are missing on $s; syncing..." >&2
    rsync -a /home/isucon/webapp/preset-images/ $s:/home/isucon/webapp/preset-images/
  fi
  ssh $s bash -l -c "bin/build.sh"
  ssh $s "sudo nginx -t && sudo systemctl reload nginx"
  ssh $s "rm -rf /home/isucon/webapp/cache/images/*"
done

# ローカルのnginx設定もマスターに合わせて反映する。
sudo cp /home/isucon/isureal.conf /etc/nginx/sites-available/isureal.conf
sudo nginx -t
sudo systemctl reload nginx

# mysql
echo "SELECT pg_stat_statements_reset();" | PGPASSWORD=isureal psql -h asia2 -U isureal
#sudo truncate -s 0 "${mysql_slow_log}"
#sudo truncate -s 0 "${mysql_error_log}"
# sudo systemctl restart mysql

# nginx
sudo truncate -s 0 "${nginx_access_log}"
#sudo truncate -s 0 "${nginx_access_json}"
sudo truncate -s 0 "${nginx_error_log}"
# sudo systemctl reload nginx


(
  dstat -tlamp -no-color 1 90 > /result/dstat-asia1.txt &
  ssh asia2 dstat -tlamp -no-color 1 90 > /result/dstat-asia2.txt &
)
echo "OK"
