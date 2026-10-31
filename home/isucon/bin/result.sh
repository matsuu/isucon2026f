#!/bin/sh

################################################################################
echo "# Analyze"
################################################################################

. /tmp/prepared_env

#mkdir -p "${data_dir}"
sudo journalctl --since="${prepared_time}" | gzip -9c > "${data_dir}/journal.log.gz"
sudo cat "${nginx_access_log}" | gzip -9c > "${data_dir}/nginx_access.log.gz"
sudo cat "${nginx_error_log}" | gzip -9c > "${data_dir}/nginx_error.log.gz"

${HOME}/go/bin/slowquery2tsv -D postgres -h asia2 -u isureal -p isureal > "${result_dir}/db.tsv"

sudo cat "${nginx_access_log}" | kataribe -f "${HOME}/kataribe.toml" > "${result_dir}/kataribe.txt"
#sudo cat "${mysql_slow_log}" | go-mysql-query-digest --limit 100% > "${result_dir}/pt-query-digest.txt"

sudo git add /
sudo git status

set -e

echo -n "ok?(Y/n): " ; read ok
if test "$ok" != "" -a "$ok" != "y" -a "$ok" != "Y" ; then
  echo "exit"
  exit 1
fi
echo -n "score: " ; read score
echo -n "comment: " ; read comment

set -ex

sudo git commit --allow-empty -m "$score $comment"
hash=`sudo git rev-parse HEAD`
sudo git push origin HEAD

#cat "${data_dir}/import.sql" | sed -e "s/REPLACE_HERE/$hash/" | duckdb --cmd "set variable score = $score; set variable comment = '$comment'; set variable hash='$hash';" "${data_dir}/isucon14.duckdb"

#sudo cp "${data_dir}/isucon14.duckdb" "${data_dir}/evidence/sources/isucon14/"
#(
#  cd "${data_dir}/evidence/"
#  sudo git add ./
#  sudo git commit -m "$score $comment"
#  sudo git push origin HEAD
#)
